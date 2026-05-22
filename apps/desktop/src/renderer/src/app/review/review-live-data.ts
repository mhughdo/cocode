import { useEffect, useRef, useState } from "react";

import {
  type ApiClient,
  errorApiState,
  type FindingListResponse,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  preserveSuccessfulLoadable,
  type ReviewEvent,
  type ReviewSession,
  type ReviewSessionSummary,
  successApiState,
} from "@/lib/api";

const MAX_REVIEW_EVENTS_RENDERED = 20000;
const MAX_REVIEW_EVENTS_PER_AGENT_RUN = 2500;
const MAX_NON_AGENT_RUN_EVENTS = 1200;

export type ReviewRefreshState =
  | { status: "idle" }
  | { status: "refreshing" }
  | { status: "error"; message: string };

export function useReviewSessionLiveData(
  client: ApiClient | null,
  initialSession?: ReviewSession,
) {
  const [session, setSession] = useState<ReviewSession | undefined>(
    initialSession,
  );
  const [summary, setSummary] =
    useState<Loadable<ReviewSessionSummary>>(idleApiState());
  const [findings, setFindings] =
    useState<Loadable<FindingListResponse>>(idleApiState());
  const [events, setEvents] = useState<ReviewEvent[]>([]);
  const [refreshState, setRefreshState] = useState<ReviewRefreshState>({
    status: "idle",
  });
  const [streamState, setStreamState] =
    useState<Loadable<true>>(idleApiState());
  const sessionStatusRef = useRef(initialSession?.status);
  const initialSessionRef = useRef(initialSession);

  useEffect(() => {
    sessionStatusRef.current = session?.status;
  }, [session?.status]);

  useEffect(() => {
    initialSessionRef.current = initialSession;
  }, [initialSession]);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (!canceled) {
        setSession(initialSessionRef.current);
        setEvents([]);
        setRefreshState({ status: "idle" });
        setStreamState(idleApiState());
      }
    });
    return () => {
      canceled = true;
    };
  }, [initialSession?.id]);

  useEffect(() => {
    if (!client || !initialSession) {
      return;
    }
    const api = client;
    const sessionId = initialSession.id;
    let canceled = false;

    async function load(initialLoad = false) {
      if (initialLoad) {
        setSummary((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
        setFindings((current) =>
          current.status === "success" ? current : loadingApiState(),
        );
      } else {
        setRefreshState({ status: "refreshing" });
      }
      const [summaryState, findingsState] = await Promise.all([
        loadApiResource(() => api.reviewSessionSummary(sessionId)),
        loadApiResource(() => api.listFindings(sessionId)),
      ]);
      if (canceled) {
        return;
      }
      setSummary((current) =>
        preserveSuccessfulLoadable(current, summaryState),
      );
      setFindings((current) =>
        preserveSuccessfulLoadable(current, findingsState),
      );
      const refreshErrors = [summaryState, findingsState]
        .filter((state) => state.status === "error")
        .map((state) => (state.status === "error" ? state.error.message : ""));
      setRefreshState(
        refreshErrors.length > 0
          ? { status: "error", message: refreshErrors.join(" ") }
          : { status: "idle" },
      );
    }

    queueMicrotask(() => {
      if (!canceled) {
        void load(true);
      }
    });
    const interval = window.setInterval(() => {
      if (isActiveReviewStatus(sessionStatusRef.current)) {
        void load();
      }
    }, 2500);
    return () => {
      canceled = true;
      window.clearInterval(interval);
    };
  }, [client, initialSession]);

  useEffect(() => {
    if (!client || !initialSession) {
      return;
    }
    const api = client;
    const sessionId = initialSession.id;
    const controller = new AbortController();
    let refreshTimer: number | undefined;
    let refreshInFlight = false;
    let refreshNeedsFindings = false;
    let refreshNeedsSession = false;
    let refreshAgain = false;

    const flushEventRefresh = async () => {
      refreshTimer = undefined;
      if (refreshInFlight) {
        refreshAgain = true;
        return;
      }
      refreshInFlight = true;
      const shouldLoadFindings = refreshNeedsFindings;
      const shouldLoadSession = refreshNeedsSession;
      refreshNeedsFindings = false;
      refreshNeedsSession = false;
      const [sessionState, summaryState, findingsState] = await Promise.all([
        shouldLoadSession
          ? loadApiResource(() => api.getReviewSession(sessionId))
          : Promise.resolve(undefined),
        loadApiResource(() => api.reviewSessionSummary(sessionId)),
        shouldLoadFindings
          ? loadApiResource(() => api.listFindings(sessionId))
          : Promise.resolve(undefined),
      ]);
      if (controller.signal.aborted) {
        return;
      }
      if (sessionState?.status === "success") {
        setSession(sessionState.data);
      }
      setSummary((current) =>
        preserveSuccessfulLoadable(current, summaryState),
      );
      if (findingsState) {
        setFindings((current) =>
          preserveSuccessfulLoadable(current, findingsState),
        );
      }
      refreshInFlight = false;
      if (refreshAgain || refreshNeedsFindings || refreshNeedsSession) {
        refreshAgain = false;
        refreshTimer = window.setTimeout(() => void flushEventRefresh(), 150);
      }
    };

    const scheduleEventRefresh = ({
      findings: includeFindings,
      session: includeSession,
    }: {
      findings?: boolean;
      session?: boolean;
    }) => {
      refreshNeedsFindings = refreshNeedsFindings || Boolean(includeFindings);
      refreshNeedsSession = refreshNeedsSession || Boolean(includeSession);
      if (refreshTimer === undefined) {
        refreshTimer = window.setTimeout(() => void flushEventRefresh(), 150);
      }
    };

    queueMicrotask(() => {
      if (!controller.signal.aborted) {
        setStreamState(loadingApiState());
      }
    });
    void api
      .streamReviewEvents(sessionId, {
        signal: controller.signal,
        onEvent: (event) => {
          setStreamState(successApiState(true));
          setEvents((current) => appendBoundedEvent(current, event));
          if (
            event.type.startsWith("ReviewSession") ||
            event.type.startsWith("Finding") ||
            event.type.startsWith("AgentRun")
          ) {
            scheduleEventRefresh(reviewEventRefreshScope(event.type));
          }
        },
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setStreamState(errorApiState(error));
          setEvents((current) =>
            appendBoundedEvent(current, {
              id: "local_stream_error",
              review_session_id: sessionId,
              type: "EventStreamError",
              level: "warn",
              sequence: current.at(-1)?.sequence ?? 0,
              payload: { message: toErrorMessage(error) },
              created_at: new Date().toISOString(),
            }),
          );
        }
      });
    return () => {
      controller.abort();
      if (refreshTimer !== undefined) {
        window.clearTimeout(refreshTimer);
      }
    };
  }, [client, initialSession]);

  return {
    events,
    findings,
    refreshState,
    session,
    setSession,
    streamState,
    summary,
  };
}

function isActiveReviewStatus(status: ReviewSession["status"] | undefined) {
  return status === "queued" || status === "running" || status === "canceling";
}

export function reviewEventRefreshScope(eventType: string): {
  findings?: boolean;
  session?: boolean;
} {
  return {
    findings: eventType.startsWith("Finding") || undefined,
    session: eventType.startsWith("ReviewSession") || undefined,
  };
}

export function appendBoundedEvent(events: ReviewEvent[], event: ReviewEvent) {
  const exists = events.some(
    (candidate) =>
      candidate.id === event.id || candidate.sequence === event.sequence,
  );
  if (exists) {
    return events;
  }
  const sorted = [...events, event].sort(
    (left, right) => left.sequence - right.sequence,
  );
  if (sorted.length <= MAX_REVIEW_EVENTS_RENDERED) {
    return sorted;
  }
  return compactReviewEvents(sorted);
}

export function compactReviewEvents(events: ReviewEvent[]) {
  const kept = new Set<string>();
  const byRun = new Map<string, number>();
  let nonAgentRunEvents = 0;

  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (!event) {
      continue;
    }
    const key = event.id || String(event.sequence);
    if (event.agent_run_id && event.type.startsWith("AgentRun")) {
      const count = byRun.get(event.agent_run_id) ?? 0;
      if (
        count < MAX_REVIEW_EVENTS_PER_AGENT_RUN ||
        isAgentRunLifecycleEvent(event)
      ) {
        kept.add(key);
        byRun.set(event.agent_run_id, count + 1);
      }
      continue;
    }
    if (nonAgentRunEvents < MAX_NON_AGENT_RUN_EVENTS) {
      kept.add(key);
      nonAgentRunEvents += 1;
    }
  }

  return events.filter((event) => kept.has(event.id || String(event.sequence)));
}

function isAgentRunLifecycleEvent(event: ReviewEvent) {
  return (
    event.type === "AgentRunQueued" ||
    event.type === "AgentRunStarted" ||
    event.type === "AgentRunCompleted" ||
    event.type === "AgentRunFailed" ||
    event.type === "AgentRunCanceled"
  );
}

function toErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
