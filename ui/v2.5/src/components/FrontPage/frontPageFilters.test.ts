import { describe, expect, it } from "vitest";
import * as GQL from "src/core/generated-graphql";
import { frontPageFilterCandidates } from "./frontPageFilters";

describe("frontPageFilterCandidates", () => {
  const filters = [
    { id: "1", mode: GQL.FilterMode.Scenes },
    { id: "2", mode: GQL.FilterMode.Galleries },
    { id: "3", mode: GQL.FilterMode.Stories },
  ];

  it("leaves out story filters, which the front page can't show yet", () => {
    expect(frontPageFilterCandidates(filters, []).map((f) => f.id)).toEqual([
      "1",
      "2",
    ]);
  });

  it("leaves out filters already on the front page", () => {
    expect(frontPageFilterCandidates(filters, ["1"]).map((f) => f.id)).toEqual([
      "2",
    ]);
  });
});
