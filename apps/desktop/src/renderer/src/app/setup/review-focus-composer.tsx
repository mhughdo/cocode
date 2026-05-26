import {
  type KeyboardEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { FileTextIcon, Loader2Icon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  type ApiClient,
  type Loadable,
  type RepositoryFile,
  idleApiState,
  loadApiResource,
  loadingApiState,
} from "@/lib/api";
import { cn } from "@/lib/utils";

import { type SetupFocusFileMention, uniqueFocusFiles } from "./setup-model";

type MentionToken = {
  end: number;
  query: string;
  start: number;
};

export function ReviewFocusComposer({
  client,
  disabled = false,
  repositoryId,
  selectedFiles,
  value,
  workspaceId,
  onSelectedFilesChange,
  onValueChange,
}: {
  client: ApiClient | null;
  disabled?: boolean;
  repositoryId?: string;
  selectedFiles: SetupFocusFileMention[];
  value: string;
  workspaceId?: string;
  onSelectedFilesChange: (files: SetupFocusFileMention[]) => void;
  onValueChange: (value: string) => void;
}) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [cursorPosition, setCursorPosition] = useState(0);
  const [activeIndex, setActiveIndex] = useState(0);
  const [filesState, setFilesState] =
    useState<Loadable<RepositoryFile[]>>(idleApiState());
  const token = useMemo(
    () => currentMentionToken(value, cursorPosition),
    [cursorPosition, value],
  );
  const suggestions =
    filesState.status === "success" ? filesState.data.slice(0, 8) : [];
  const selectedPaths = useMemo(
    () => new Set(selectedFiles.map((file) => file.path)),
    [selectedFiles],
  );

  useEffect(() => {
    if (!client || !repositoryId || !workspaceId || !token) {
      return;
    }
    let canceled = false;
    const timer = window.setTimeout(() => {
      setFilesState(loadingApiState());
      void loadApiResource(() =>
        client.searchRepositoryFiles(repositoryId, {
          workspaceId,
          query: token.query,
          limit: 12,
        }),
      ).then((state) => {
        if (!canceled) {
          setFilesState(state);
        }
      });
    }, token.query === "" ? 0 : 120);
    return () => {
      canceled = true;
      window.clearTimeout(timer);
    };
  }, [client, repositoryId, token, workspaceId]);

  function updateCursor() {
    setCursorPosition(textareaRef.current?.selectionStart ?? value.length);
  }

  function selectFile(file: RepositoryFile) {
    const nextFiles = uniqueFocusFiles([...selectedFiles, file]);
    onSelectedFilesChange(nextFiles);
    if (token) {
      const before = value.slice(0, token.start);
      const after = value.slice(token.end);
      const needsSpace = before !== "" && after !== "" && !/\s$/.test(before);
      const nextValue = `${before}${needsSpace ? " " : ""}${after}`.replace(
        /[ \t]{2,}/g,
        " ",
      );
      onValueChange(nextValue);
      window.requestAnimationFrame(() => {
        const nextCursor = before.length + (needsSpace ? 1 : 0);
        textareaRef.current?.focus();
        textareaRef.current?.setSelectionRange(nextCursor, nextCursor);
        setCursorPosition(nextCursor);
      });
    }
  }

  function removeFile(path: string) {
    onSelectedFilesChange(selectedFiles.filter((file) => file.path !== path));
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (!token || suggestions.length === 0) {
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) => (current + 1) % suggestions.length);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex(
        (current) => (current - 1 + suggestions.length) % suggestions.length,
      );
      return;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      selectFile(suggestions[activeIndex] ?? suggestions[0]);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      setFilesState(idleApiState());
    }
  }

  return (
    <div className="relative">
      <PopoverPrimitive.Root open={Boolean(token)}>
        <PopoverPrimitive.Anchor asChild>
          <div className="border-border-subtle bg-card focus-within:border-foreground/30 rounded-xl border shadow-[0_1px_2px_rgb(17_18_20/0.04)]">
            {selectedFiles.length > 0 && (
              <div className="border-border-subtle flex flex-wrap gap-1.5 border-b px-2.5 py-2">
                {selectedFiles.map((file) => (
                  <span
                    key={file.path}
                    className="bg-primary/5 text-primary border-primary/15 inline-flex max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium"
                  >
                    <FileTextIcon className="size-3.5 shrink-0" />
                    <span className="min-w-0 truncate">{file.path}</span>
                    <button
                      aria-label={`Remove ${file.path}`}
                      className="hover:bg-primary/10 -mr-1 flex size-5 shrink-0 cursor-pointer items-center justify-center rounded"
                      type="button"
                      onClick={() => removeFile(file.path)}
                    >
                      <XIcon className="size-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
            <Textarea
              ref={textareaRef}
              aria-label="Review context"
              className="min-h-[82px] resize-none border-0 bg-transparent px-3 py-2.5 text-[0.84rem] leading-5 shadow-none focus-visible:ring-0"
              disabled={disabled}
              placeholder="Pay attention to @docs/prd.md, reward accounting, and rollback paths..."
              value={value}
              onBlur={updateCursor}
              onChange={(event) => {
                onValueChange(event.target.value);
                setCursorPosition(event.target.selectionStart);
                setActiveIndex(0);
              }}
              onClick={updateCursor}
              onKeyDown={handleKeyDown}
              onKeyUp={updateCursor}
              onSelect={updateCursor}
            />
          </div>
        </PopoverPrimitive.Anchor>

        {token && (
          <PopoverPrimitive.Portal>
            <PopoverPrimitive.Content
              align="start"
              avoidCollisions
              className="bg-popover border-border-subtle z-[80] max-h-[min(18rem,var(--radix-popover-content-available-height))] w-[var(--radix-popover-trigger-width)] overflow-hidden rounded-xl border p-1.5 shadow-lg"
              collisionPadding={12}
              side="bottom"
              sideOffset={8}
              onOpenAutoFocus={(event) => event.preventDefault()}
            >
              <div className="flex items-center justify-between gap-2 px-2 py-1.5">
                <span className="text-muted-foreground text-xs font-medium">
                  Mention file
                </span>
                {filesState.status === "loading" && (
                  <Loader2Icon className="text-muted-foreground size-3.5 animate-spin" />
                )}
              </div>
              <div className="max-h-48 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                {suggestions.map((file, index) => (
                  <button
                    key={file.path}
                    className={cn(
                      "flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-left transition-colors",
                      index === activeIndex && "bg-surface-muted",
                    )}
                    type="button"
                    onMouseDown={(event) => event.preventDefault()}
                    onMouseEnter={() => setActiveIndex(index)}
                    onClick={() => selectFile(file)}
                  >
                    <FileTextIcon className="text-muted-foreground size-4 shrink-0" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {file.name}
                      </span>
                      {file.directory && (
                        <span className="text-muted-foreground block truncate text-xs">
                          {file.directory}
                        </span>
                      )}
                    </span>
                    {selectedPaths.has(file.path) && (
                      <span className="text-muted-foreground text-xs">
                        Added
                      </span>
                    )}
                  </button>
                ))}
                {filesState.status === "success" && suggestions.length === 0 && (
                  <p className="text-muted-foreground px-2.5 py-4 text-sm">
                    No files match @{token.query}
                  </p>
                )}
                {filesState.status === "error" && (
                  <p className="text-destructive px-2.5 py-4 text-sm">
                    {filesState.error.message}
                  </p>
                )}
              </div>
            </PopoverPrimitive.Content>
          </PopoverPrimitive.Portal>
        )}
      </PopoverPrimitive.Root>

      {selectedFiles.length > 0 && (
        <div className="text-muted-foreground mt-1.5 flex items-center justify-end gap-2 text-[0.72rem]">
          <Button
            className="h-6 px-2 text-[0.68rem]"
            size="sm"
            variant="ghost"
            onClick={() => onSelectedFilesChange([])}
          >
            Clear files
          </Button>
        </div>
      )}
    </div>
  );
}

function currentMentionToken(text: string, cursor: number): MentionToken | null {
  const boundedCursor = Math.max(0, Math.min(cursor, text.length));
  const prefix = text.slice(0, boundedCursor);
  const at = prefix.lastIndexOf("@");
  if (at < 0) {
    return null;
  }
  if (at > 0 && !/\s/.test(text[at - 1])) {
    return null;
  }
  const query = text.slice(at + 1, boundedCursor);
  if (/\s/.test(query)) {
    return null;
  }
  return { end: boundedCursor, query, start: at };
}
