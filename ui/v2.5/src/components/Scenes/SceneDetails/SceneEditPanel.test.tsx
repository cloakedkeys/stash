import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps, ReactNode } from "react";
import * as GQL from "src/core/generated-graphql";
import { SceneEditPanel } from "./SceneEditPanel";
import { SceneFileRangeFields } from "./SceneFileRange";

const page = vi.hoisted(() => ({
  values: undefined as undefined | Record<string, unknown>,
  submit: undefined as undefined | (() => void),
  range: undefined as undefined | ComponentProps<typeof SceneFileRangeFields>,
  onSubmit: vi.fn(),
  error: vi.fn(),
}));

vi.mock("formik", () => ({
  useFormik: (options: {
    initialValues: Record<string, unknown>;
    onSubmit: (values: Record<string, unknown>) => void;
  }) => {
    page.values ??= { ...options.initialValues };
    page.submit = () => options.onSubmit(page.values!);
    return {
      values: page.values,
      initialValues: options.initialValues,
      dirty: true,
      errors: {},
      setFieldValue: (field: string, value: unknown) => {
        page.values![field] = value;
      },
      submitForm: page.submit,
      resetForm: vi.fn(),
    };
  },
}));
vi.mock("./SceneFileRange", () => ({
  SceneFileRangeFields: (
    props: ComponentProps<typeof SceneFileRangeFields>
  ) => {
    page.range = props;
    return null;
  },
}));
vi.mock("src/hooks/CurrentUser", () => ({
  useCanAdminister: () => false,
}));
vi.mock("src/hooks/Config", () => ({
  useConfigurationContext: () => ({ configuration: undefined }),
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: page.error }),
}));
vi.mock("react-router-dom", () => ({ Prompt: () => null }));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
  FormattedMessage: () => null,
}));
vi.mock("mousetrap", () => ({ default: {} }));
vi.mock("src/utils/lazyComponent", () => ({
  lazyComponent: () => () => null,
}));
vi.mock("src/core/StashService", () => ({
  useListSceneScrapers: () => ({ data: undefined }),
}));
vi.mock("src/utils/image", () => ({
  default: { usePasteImage: () => undefined },
}));
vi.mock("src/utils/stashIds", () => ({
  getStashIDs: () => [],
}));
vi.mock("src/utils/yup", async () => {
  const yup = await import("yup");
  return {
    yupUniqueStringList: () => yup.array(yup.string()).defined(),
    yupDateString: () => yup.string(),
    yupFormikValidate: () => () => ({}),
  };
});
vi.mock("src/utils/form", () => ({
  formikUtils: () => ({
    renderField: (_field: string, _title: string, control: ReactNode) =>
      control,
    renderInputField: () => null,
    renderDateField: () => null,
    renderURLListField: () => null,
    renderStashIDsField: () => null,
  }),
}));
vi.mock("src/hooks/tagsEdit", () => ({
  useTagsEdit: () => ({
    tags: [],
    tagsControl: () => null,
    markTagsStateSaved: vi.fn(),
  }),
}));
vi.mock("./usePerformerPictureFromFrame", () => ({
  usePerformerPictureFromFrame: () => ({}),
}));
vi.mock("src/components/Shared/CustomFields", () => ({
  CustomFieldsInput: () => null,
  formatCustomFieldInput: () => ({}),
}));
vi.mock("src/components/Shared/Icon", () => ({ Icon: () => null }));
vi.mock("src/components/Shared/ImageInput", () => ({
  ImageInput: () => null,
}));
vi.mock("src/components/Shared/ScraperMenu", () => ({
  ScraperMenu: () => null,
}));
vi.mock("src/components/Shared/StashBoxIDSearchModal", () => ({
  default: () => null,
}));
vi.mock("src/components/Performers/PerformerSelect", () => ({
  PerformerSelect: () => null,
}));
vi.mock("src/components/VoiceActors/VoiceActorSelect", () => ({
  VoiceActorSelect: () => null,
}));
vi.mock("src/components/Studios/StudioSelect", () => ({
  StudioSelect: () => null,
}));
vi.mock("src/components/Galleries/GallerySelect", () => ({
  GallerySelect: () => null,
}));
vi.mock("./SceneGroupTable", () => ({
  SceneGroupTable: () => null,
}));

const file = { id: "7" } as GQL.VideoFileDataFragment;

function render(scene: Partial<GQL.SceneDataFragment>) {
  page.range = undefined;
  renderToStaticMarkup(
    <SceneEditPanel scene={scene} isVisible onSubmit={page.onSubmit} />
  );
}

beforeEach(() => {
  page.values = undefined;
  page.range = undefined;
  vi.clearAllMocks();
  page.onSubmit.mockResolvedValue(undefined);
});

describe("scene edit file times", () => {
  it("loads the saved times and saves edits through the scene input", () => {
    render({ files: [file], start_time: 83, end_time: 296 });
    expect(page.range?.startTime).toBe(83);
    expect(page.range?.endTime).toBe(296);
    page.range!.onStartTimeChange(90.25);
    page.range!.onEndTimeChange(300);
    page.submit!();
    expect(page.onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ start_time: 90.25, end_time: 300 }),
      undefined
    );
  });

  it("uses the loaded scene start for the player even after editing the start", () => {
    const scene = { files: [file], start_time: 83, end_time: 296 };
    render(scene);
    page.range!.onStartTimeChange(100);
    render(scene);
    expect(page.range?.startTime).toBe(100);
    expect(page.range?.playerStartTime).toBe(83);
  });

  it("sends explicit nulls when both times are cleared", () => {
    render({ files: [file], start_time: 83, end_time: 296 });
    page.range!.onStartTimeChange(null);
    page.range!.onEndTimeChange(null);
    page.submit!();
    expect(page.onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ start_time: null, end_time: null }),
      undefined
    );
  });

  it("saves a start with an open end", () => {
    render({ files: [file], start_time: null, end_time: null });
    page.range!.onStartTimeChange(90);
    page.submit!();
    expect(page.onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ start_time: 90, end_time: null }),
      undefined
    );
  });

  it.each([
    { files: [] },
    { files: [file, { ...file, id: "8" }] },
  ])("hides range fields without exactly one file: %j", ({ files }) => {
    render({ files });
    expect(page.range).toBeUndefined();
    page.submit!();
    const input = page.onSubmit.mock.calls[0][0];
    expect(input.start_time).toBeUndefined();
    expect(input.end_time).toBeUndefined();
  });

  it("surfaces a server refusal", async () => {
    const error = new Error("start time must be before the end time");
    page.onSubmit.mockRejectedValue(error);
    render({ files: [file], start_time: 83, end_time: 296 });
    page.range!.onStartTimeChange(500);
    page.submit!();
    await vi.waitFor(() => expect(page.error).toHaveBeenCalledWith(error));
  });
});
