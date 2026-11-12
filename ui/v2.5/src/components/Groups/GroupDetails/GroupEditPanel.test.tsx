import { renderToStaticMarkup } from "react-dom/server";
import { expect, it, vi } from "vitest";
import { GroupEditPanel } from "./GroupEditPanel";

const page = vi.hoisted(() => ({ values: {} as Record<string, unknown> }));
vi.mock("formik", () => ({
  useFormik: (options: { initialValues: Record<string, unknown> }) => {
    page.values = options.initialValues;
    return {
      values: page.values,
      errors: {},
      dirty: false,
      setFieldValue: vi.fn(),
    };
  },
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({ formatMessage: ({ id }: { id: string }) => id }),
  FormattedMessage: () => null,
}));
vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/groups/new" }),
  Prompt: () => null,
}));
vi.mock("src/core/StashService", () => ({
  useListGroupScrapers: () => ({}),
  queryScrapeGroupURL: vi.fn(),
}));
vi.mock("src/hooks/Toast", () => ({ useToast: () => ({ error: vi.fn() }) }));
vi.mock("src/utils/image", () => ({ default: { usePasteImage: () => false } }));
vi.mock("src/utils/form", () => ({
  useValidateAfterChange: () => vi.fn(),
  formikUtils: () => ({
    renderField: () => null,
    renderInputField: () => null,
    renderDateField: () => null,
    renderDurationField: () => null,
    renderStringListField: () => null,
    renderURLListField: () => null,
  }),
}));
vi.mock("src/hooks/tagsEdit", () => ({
  useTagsEdit: () => ({ tagsControl: () => null }),
}));
vi.mock("src/components/Studios/StudioSelect", () => ({
  StudioSelect: () => null,
}));
vi.mock("./RelatedGroupTable", () => ({ RelatedGroupTable: () => null }));
vi.mock("./GroupScrapeDialog", () => ({ GroupScrapeDialog: () => null }));
vi.mock("src/components/Shared/CustomFields", () => ({
  CustomFieldsInput: () => null,
}));
vi.mock("src/components/Shared/DetailsEditNavbar", () => ({
  DetailsEditNavbar: () => null,
}));

it("includes the supplied front cover in the form's initial save values", () => {
  renderToStaticMarkup(
    <GroupEditPanel
      group={{
        name: "Movie",
        date: "2020-08-31",
        director: "Director",
        studio: {
          id: "2",
          name: "Studio",
          aliases: [],
          favorite: false,
          ignore_auto_tag: false,
          organized: false,
          o_counter: 0,
          stash_ids: [],
          tags: [],
        },
        urls: ["https://example.com/movie"],
        front_image: "data:image/jpeg;base64,Y292ZXI=",
      }}
      onSubmit={vi.fn()}
      onCancel={vi.fn()}
      onDelete={vi.fn()}
      setFrontImage={vi.fn()}
      setBackImage={vi.fn()}
      setEncodingImage={vi.fn()}
    />
  );
  expect(page.values).toMatchObject({
    name: "Movie",
    date: "2020-08-31",
    director: "Director",
    studio_id: "2",
    urls: ["https://example.com/movie"],
    front_image: "data:image/jpeg;base64,Y292ZXI=",
  });
  renderToStaticMarkup(
    <GroupEditPanel
      group={{ name: "", date: null, director: null, studio: null, urls: [] }}
      onSubmit={vi.fn()}
      onCancel={vi.fn()}
      onDelete={vi.fn()}
      setFrontImage={vi.fn()}
      setBackImage={vi.fn()}
      setEncodingImage={vi.fn()}
    />
  );
  expect(page.values).toMatchObject({
    name: "",
    date: "",
    director: "",
    studio_id: null,
    urls: [],
    front_image: undefined,
  });
});
