import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { Kind, parse, visit } from "graphql";
import { addPerformerImageFiles, movePerformerImage } from "./performerImages";

afterEach(() => vi.unstubAllGlobals());

it("requests alternate images only in the detail query, never shared list fragments", () => {
  for (const name of ["performer", "performer-slim"]) {
    const fragment = readFileSync(
      new URL(`../../../../graphql/data/${name}.graphql`, import.meta.url),
      "utf8"
    );
    expect(fragment).not.toContain("alternate_images");
  }
  const queries = parse(
    readFileSync(
      new URL("../../../../graphql/queries/performer.graphql", import.meta.url),
      "utf8"
    )
  );
  const requesting: string[] = [];
  for (const definition of queries.definitions) {
    if (definition.kind !== Kind.OPERATION_DEFINITION) continue;
    visit(definition, {
      Field(field) {
        if (field.name.value === "alternate_images")
          requesting.push(definition.name!.value);
      },
    });
  }
  expect(requesting).toEqual(["FindPerformer"]);
});

describe("performer image ordering", () => {
  it("moves in either direction without changing the input", () => {
    const images = ["a", "b", "c"];
    expect(movePerformerImage(images, 1, -1)).toEqual(["b", "a", "c"]);
    expect(movePerformerImage(images, 1, 1)).toEqual(["a", "c", "b"]);
    expect(images).toEqual(["a", "b", "c"]);
  });

  it("leaves boundary and missing images alone", () => {
    expect(movePerformerImage([], 0, 1)).toEqual([]);
    expect(movePerformerImage(["a"], 0, -1)).toEqual(["a"]);
    expect(movePerformerImage(["a"], 0, 1)).toEqual(["a"]);
    expect(movePerformerImage(["a"], -1, 1)).toEqual(["a"]);
    expect(movePerformerImage(["a"], 1, -1)).toEqual(["a"]);
  });

  it("keeps the complete order when moving across a thumbnail page boundary", () => {
    const images = Array.from({ length: 25 }, (_, index) => String(index));
    const moved = movePerformerImage(images, 11, 1);
    expect(moved.slice(10, 14)).toEqual(["10", "12", "11", "13"]);
    expect(moved).toHaveLength(images.length);
    expect(new Set(moved)).toEqual(new Set(images));
  });
});

describe("performer file additions", () => {
  it("reads and adds serially, reports each failure, and continues", async () => {
    const events: string[] = [];
    vi.stubGlobal(
      "FileReader",
      class {
        result = "";
        error = new Error("read failed");
        onload = () => {};
        onerror = () => {};
        readAsDataURL(file: File) {
          events.push(`read:${file.name}`);
          this.result = file.name;
          if (file.name === "unreadable") this.onerror();
          else this.onload();
        }
      }
    );
    const files = ["first", "rejected", "unreadable", "last"].map(
      (name) => new File([name], name)
    );
    const failures: string[] = [];
    await addPerformerImageFiles(
      files,
      async (image) => {
        events.push(`add:${image}`);
        await Promise.resolve();
        if (image === "rejected") throw new Error("upload failed");
        events.push(`done:${image}`);
      },
      (file) => failures.push(file.name)
    );
    expect(events).toEqual([
      "read:first",
      "add:first",
      "done:first",
      "read:rejected",
      "add:rejected",
      "read:unreadable",
      "read:last",
      "add:last",
      "done:last",
    ]);
    expect(failures).toEqual(["rejected", "unreadable"]);
  });
});
