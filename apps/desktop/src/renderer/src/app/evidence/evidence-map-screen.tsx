import { useEffect, useState } from "react";
import { RefreshCwIcon } from "lucide-react";

import { ErrorState, LoadingRows } from "@/components/app/chrome";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  type ApiClient,
  errorApiState,
  type EvidenceMapResponse,
  type Finding,
  idleApiState,
  type Loadable,
  loadApiResource,
  loadingApiState,
  type Repository,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { ReviewBreadcrumb } from "../shared/review-breadcrumb";
import { panelMotionClass, usePanelPresence } from "../shared/panel-motion";
import {
  EvidenceMapGraphCanvas,
  firstEvidenceMapSelection,
  type EvidenceMapSelection,
} from "./review-evidence-map";
import { truncate } from "./review-evidence-utils";
import { EvidenceMapInspectorPanel } from "./evidence-map-inspector-panel";

export function EvidenceMapScreen({
  activeRepository,
  client,
  finding,
  globalRightPanelOpen,
  onBack,
  onOpenFindingDetail,
}: {
  activeRepository?: Repository;
  client: ApiClient | null;
  finding: Finding;
  globalRightPanelOpen?: boolean;
  onBack: () => void;
  onOpenFindingDetail: (finding: Finding) => void;
}) {
  const [mapState, setMapState] =
    useState<Loadable<EvidenceMapResponse>>(idleApiState());
  const [selection, setSelection] = useState<EvidenceMapSelection | null>(null);
  const [actionMessage, setActionMessage] = useState("");
  const [isRebuilding, setIsRebuilding] = useState(false);

  useEffect(() => {
    let canceled = false;
    queueMicrotask(() => {
      if (canceled) {
        return;
      }
      if (!client) {
        setMapState(errorApiState(new Error("Backend client is unavailable")));
        return;
      }
      setMapState(loadingApiState());
      setSelection(null);
      setActionMessage("");
      void loadApiResource(() => client.getFindingEvidenceMap(finding.id)).then(
        (state) => {
          if (canceled) {
            return;
          }
          setMapState(state);
          if (state.status === "success") {
            setSelection(firstEvidenceMapSelection(state.data));
          }
        },
      );
    });
    return () => {
      canceled = true;
    };
  }, [client, finding.id]);

  const map = mapState.status === "success" ? mapState.data : undefined;
  const selectedNode =
    map && selection?.kind === "node"
      ? map.nodes.find((node) => node.id === selection.id)
      : undefined;
  const selectedEdge =
    map && selection?.kind === "edge"
      ? map.edges.find((edge) => edge.id === selection.id)
      : undefined;
  const selectedCallPath =
    map && selection?.kind === "call_path"
      ? map.call_paths.find((path) => path.id === selection.id)
      : undefined;
  const displayFinding = map?.finding ?? finding;
  const inspectorPresence = usePanelPresence(
    Boolean(map && !globalRightPanelOpen),
  );
  const inspectorLayoutActive = Boolean(map && inspectorPresence.rendered);
  const inspectorVisible = inspectorPresence.visible && !globalRightPanelOpen;

  async function rebuildMap() {
    if (!client) {
      setActionMessage("Backend client is unavailable.");
      return;
    }
    setIsRebuilding(true);
    setActionMessage("");
    const state = await loadApiResource(() =>
      client.rebuildFindingEvidenceMap(finding.id),
    );
    setIsRebuilding(false);
    setMapState(state);
    if (state.status === "success") {
      setSelection(firstEvidenceMapSelection(state.data));
      setActionMessage("Evidence Map rebuilt");
      return;
    }
    setActionMessage(
      state.status === "error" ? state.error.message : "Rebuild failed",
    );
  }

  return (
    <div className="flex h-full min-h-[calc(100vh-220px)] min-w-0 flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <ReviewBreadcrumb
            items={[
              { label: "Findings", onClick: onBack },
              {
                label: truncate(displayFinding.canonical_claim, 88),
                onClick: () => onOpenFindingDetail(finding),
              },
              { label: "Evidence map" },
            ]}
          />
          <h2 className="text-2xl leading-8 font-semibold">Evidence Map</h2>
          <p className="text-muted-foreground mt-1 max-w-3xl text-sm break-words">
            {displayFinding.canonical_claim}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            disabled={mapState.status !== "success" || isRebuilding}
            size="sm"
            variant="outline"
            onClick={() => void rebuildMap()}
          >
            <RefreshCwIcon data-icon="inline-start" />
            Rebuild
          </Button>
        </div>
      </div>

      {mapState.status === "loading" && (
        <LoadingRows rows={8} className="cocode-panel p-4" />
      )}
      {mapState.status === "error" && (
        <ErrorState
          title="Evidence Map failed to load"
          description={mapState.error.message}
        />
      )}
      {map && (
        <div
          className={cn(
            "cocode-panel min-h-0 min-w-0 flex-1 overflow-hidden transition-[grid-template-columns]",
            panelMotionClass,
            inspectorLayoutActive
              ? "evidence-map-layout"
              : "grid h-full grid-cols-1",
          )}
        >
          <div className="flex min-h-0 min-w-0 flex-col bg-white">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="mr-1 text-sm font-semibold">
                  Evidence flow
                </span>
                <Badge
                  variant={map.graph.status === "ready" ? "default" : "outline"}
                >
                  {map.graph.status}
                </Badge>
                <Badge variant="secondary">{map.nodes.length} nodes</Badge>
                <Badge variant="secondary">{map.edges.length} edges</Badge>
                {map.call_paths.length > 0 && (
                  <Badge variant="outline">
                    {map.call_paths.length} call path
                    {map.call_paths.length === 1 ? "" : "s"}
                  </Badge>
                )}
              </div>
              {actionMessage && (
                <span className="text-muted-foreground text-xs">
                  {actionMessage}
                </span>
              )}
            </div>

            {map.missing_reasons && map.missing_reasons.length > 0 && (
              <ErrorState
                className="m-4"
                title="Evidence Map is partial"
                description={map.missing_reasons.join(" ")}
              />
            )}

            <div className="min-h-0 flex-1 overflow-hidden">
              <EvidenceMapGraphCanvas
                map={map}
                selection={selection}
                onSelect={setSelection}
              />
            </div>
          </div>

          {inspectorLayoutActive && (
            <div
              aria-hidden={!inspectorVisible}
              className={cn(
                "min-h-0 min-w-0 transform-gpu transition-[opacity,transform] will-change-transform",
                panelMotionClass,
                inspectorVisible
                  ? "translate-x-0 opacity-100"
                  : "pointer-events-none translate-x-8 opacity-0",
              )}
            >
              <EvidenceMapInspectorPanel
                activeRepositoryPath={activeRepository?.local_path}
                map={map}
                selectedCallPath={selectedCallPath}
                selectedEdge={selectedEdge}
                selectedNode={selectedNode}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
