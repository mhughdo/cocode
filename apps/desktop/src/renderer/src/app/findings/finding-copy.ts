import type { Finding, FindingDetailResponse } from "@/lib/api";
import {
  evidenceCodeSnippet,
  formatDecisionLabel,
  formatEvidenceLocation,
  formatFindingLocation,
  prioritizedEvidenceItems,
  snippetPreview,
} from "../evidence/review-evidence-utils";

export function findingClipboardText(finding: Finding) {
  return detailedFindingDraftComment(finding);
}

export function detailedFindingDraftComment(
  finding: Finding,
  detail?: FindingDetailResponse,
) {
  const lines = [
    `### ${finding.canonical_claim}`,
    "",
    `**Severity:** ${formatDecisionLabel(finding.severity)}`,
    `**Confidence:** ${Math.round(finding.confidence * 100)}%`,
    `**Status:** ${formatDecisionLabel(finding.verification_status)}`,
    `**Location:** \`${formatFindingLocation(finding)}\``,
    "",
    finding.evidence_summary,
  ].filter(Boolean) as string[];

  const evidenceItems = prioritizedEvidenceItems(detail?.evidence_items ?? []);
  if (evidenceItems.length > 0) {
    lines.push("", "#### Evidence");
    evidenceItems.slice(0, 5).forEach((item) => {
      lines.push(
        `- **${item.title}** (${formatEvidenceLocation(item)}, ${Math.round(
          item.confidence * 100,
        )}%): ${item.summary}`,
      );
      const codeSnippet = evidenceCodeSnippet(item);
      if (codeSnippet) {
        lines.push("", "```");
        lines.push(snippetPreview(codeSnippet, 12));
        lines.push("```");
      }
    });
  }

  if (finding.suggested_fix) {
    lines.push("", "#### Suggested fix", finding.suggested_fix);
  }

  const dedupeReason = (finding as Finding & { dedupe_reason?: string })
    .dedupe_reason;
  if (dedupeReason) {
    lines.push("", "#### Deduplication", dedupeReason);
  }

  return `${lines.join("\n")}\n`;
}
