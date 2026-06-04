import type { ChangedFile, Snapshot } from "@/lib/api";

export type SetupSourcePreview = {
  key: string;
  snapshot: Snapshot;
  files: ChangedFile[];
};

export type SetupPreviewStats = {
  total: number;
  reviewable: number;
  additions: number;
  deletions: number;
  generated: number;
  binary: number;
  excluded: number;
};

import { panelMotionDurationMs } from "../shared/panel-motion";

export const sourceInspectorMinWidth = 380;
export const sourceInspectorDefaultWidth = 760;
export const sourceInspectorMaxWidth = 1280;
export const sourceInspectorMainMinWidth = 520;
export const sourceInspectorOverlayGutter = 16;
export const sourceInspectorSideBySideMinWidth = 1180;
export const sourceInspectorTransitionMs = panelMotionDurationMs;
export const setupInitialDiffFileRenderCount = 6;
export const setupDiffFileRenderBatchSize = 6;
export const setupMaxRenderedDiffFiles = 200;

export function setupPreviewStats(files: ChangedFile[]): SetupPreviewStats {
  return files.reduce<SetupPreviewStats>(
    (stats, file) => ({
      total: stats.total + 1,
      reviewable:
        stats.reviewable + (file.is_binary || file.is_excluded ? 0 : 1),
      additions: stats.additions + file.additions,
      deletions: stats.deletions + file.deletions,
      generated: stats.generated + (file.is_generated ? 1 : 0),
      binary: stats.binary + (file.is_binary ? 1 : 0),
      excluded: stats.excluded + (file.is_excluded ? 1 : 0),
    }),
    {
      total: 0,
      reviewable: 0,
      additions: 0,
      deletions: 0,
      generated: 0,
      binary: 0,
      excluded: 0,
    },
  );
}
