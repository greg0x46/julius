import { describe, expect, it } from "vitest";

import { getFailureStageLabel, getStatusMetadata, isFinalStatus } from "./syncStatus";
import { failureStages, syncStatuses } from "../api/contracts";

describe("sync status presentation", () => {
  it("labels every contracted status in pt-BR with a semantic symbol", () => {
    expect(syncStatuses.map((status) => getStatusMetadata(status)).map(({ label }) => label)).toEqual([
      "Em andamento",
      "Concluída",
      "Concluída com falhas",
      "Falhou",
    ]);
    expect(syncStatuses.every((status) => getStatusMetadata(status).symbol.length > 0)).toBe(true);
  });

  it("translates status and failure stage labels", () => {
    expect(getStatusMetadata("completed_with_failures", "en").label).toBe("Completed with failures");
    expect(getFailureStageLabel("accounts", "en")).toBe("Account listing");
  });

  it("identifies final states and labels every failure stage", () => {
    expect(isFinalStatus("in_progress")).toBe(false);
    expect(syncStatuses.slice(1).every(isFinalStatus)).toBe(true);
    expect(failureStages.every((stage) => getFailureStageLabel(stage).length > 0)).toBe(true);
  });
});
