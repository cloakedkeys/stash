import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SceneCreate from "./SceneCreate";

// The split page (#1368): renders it with its services replaced, ticks the
// box through its own handler and saves through the edit panel's onSubmit.
const page = vi.hoisted(() => ({
  search: "",
  // useState values, kept between renders in call order
  state: [] as unknown[],
  next: 0,
  check: undefined as
    | undefined
    | {
        checked: boolean;
        label: string;
        onChange: (e: { currentTarget: { checked: boolean } }) => void;
      },
  submit: undefined as
    | undefined
    | ((input: { title: string }, andNew?: boolean) => Promise<void>),
  push: vi.fn(),
  createScene: vi.fn(),
  createMarker: vi.fn(),
  success: vi.fn(),
  warning: vi.fn(),
}));

const markers = [
  {
    id: "10",
    title: "Start",
    seconds: 1.5,
    end_seconds: null,
    primary_tag: { id: "1", name: "Kiss" },
    tags: [{ id: "2", name: "Hug" }],
  },
  {
    id: "11",
    title: "Later",
    seconds: 30,
    end_seconds: 42.25,
    primary_tag: { id: "2", name: "Hug" },
    tags: [],
  },
];

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useState: (initial: unknown) => {
    const i = page.next++;
    if (!(i in page.state)) page.state[i] = initial;
    return [
      page.state[i],
      (value: unknown) => {
        page.state[i] = value;
      },
    ];
  },
  useEffect: () => {},
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({ push: page.push }),
  useLocation: () => ({ search: page.search }),
}));
vi.mock("react-intl", () => ({
  useIntl: () => ({
    formatMessage: ({ id }: { id: string }, values?: { count?: number }) =>
      values?.count !== undefined ? `${id}:${values.count}` : id,
  }),
  FormattedMessage: () => null,
}));
vi.mock("react-bootstrap", () => ({
  Form: {
    Group: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    Check: (props: NonNullable<typeof page.check>) => {
      page.check = props;
      return null;
    },
    Text: () => null,
  },
}));
vi.mock("./SceneEditPanel", () => ({
  SceneEditPanel: (props: { onSubmit: typeof page.submit }) => {
    page.submit = props.onSubmit;
    return null;
  },
}));
vi.mock("./SceneUpload", () => ({ SceneUpload: () => null }));
vi.mock("src/core/generated-graphql", () => ({}));
vi.mock("src/core/StashService", () => ({
  mutateCreateScene: page.createScene,
  useFindScene: (id: string) => ({
    loading: false,
    data:
      id === "new"
        ? undefined
        : {
            findScene: {
              id,
              title: "Original",
              paths: {},
              scene_markers: markers,
            },
          },
  }),
  useSceneMarkerCreate: () => [page.createMarker],
}));
vi.mock("src/utils/image", () => ({ default: {} }));
vi.mock("src/components/Shared/LoadingIndicator", () => ({
  LoadingIndicator: () => null,
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: page.success, toast: page.warning }),
}));
vi.mock("src/patch", () => ({
  PatchComponent: (_name: string, component: unknown) => component,
}));
vi.mock("src/hooks/CurrentUser", () => ({ useCanAdminister: () => false }));

function render() {
  page.next = 0;
  page.check = undefined;
  page.submit = undefined;
  renderToStaticMarkup(<SceneCreate />);
}

function tick() {
  page.check!.onChange({ currentTarget: { checked: true } });
  render();
}

beforeEach(() => {
  page.search = "?from_scene_id=1&file_id=7";
  page.state = [];
  vi.clearAllMocks();
  page.createScene.mockResolvedValue({ data: { sceneCreate: { id: "9" } } });
  page.createMarker.mockResolvedValue({
    data: { sceneMarkerCreate: { id: "20" } },
  });
});

describe("split page marker copy", () => {
  it("offers the original scene's markers, unticked", () => {
    render();
    expect(page.check?.label).toBe("split_scene.copy_markers:2");
    expect(page.check?.checked).toBe(false);
  });

  it("has no box for a new scene that isn't made from another", () => {
    page.search = "";
    render();
    expect(page.check).toBeUndefined();
    expect(page.submit).toBeDefined();
  });

  it("copies the markers to the new scene when ticked", async () => {
    render();
    tick();
    expect(page.check?.checked).toBe(true);

    await page.submit!({ title: "Part two" });

    expect(page.createScene).toHaveBeenCalledWith({
      title: "Part two",
      file_ids: ["7"],
    });
    expect(page.createMarker.mock.calls.map(([arg]) => arg.variables)).toEqual([
      {
        scene_id: "9",
        title: "Start",
        seconds: 1.5,
        end_seconds: null,
        primary_tag_id: "1",
        tag_ids: ["2"],
      },
      {
        scene_id: "9",
        title: "Later",
        seconds: 30,
        end_seconds: 42.25,
        primary_tag_id: "2",
        tag_ids: [],
      },
    ]);
    expect(page.push).toHaveBeenCalledWith("/scenes/9");
    expect(page.success).toHaveBeenCalled();
  });

  it("copies nothing when unticked", async () => {
    render();
    await page.submit!({ title: "Part two" });
    expect(page.createScene).toHaveBeenCalled();
    expect(page.createMarker).not.toHaveBeenCalled();
    expect(page.push).toHaveBeenCalledWith("/scenes/9");
  });

  it("stays on the page for 'save and new', with the box unticked again", async () => {
    render();
    tick();
    await page.submit!({ title: "Part two" }, true);
    expect(page.createMarker).toHaveBeenCalledTimes(2);
    expect(page.push).not.toHaveBeenCalled();
    render();
    expect(page.check?.checked).toBe(false);
  });

  it("opens the new scene with a warning when a copy fails", async () => {
    page.createMarker
      .mockResolvedValueOnce({ data: { sceneMarkerCreate: { id: "20" } } })
      .mockRejectedValueOnce(new Error("no"));
    render();
    tick();
    await page.submit!({ title: "Part two" }, true);
    expect(page.push).toHaveBeenCalledWith("/scenes/9");
    expect(page.warning).toHaveBeenCalledWith({
      variant: "warning",
      content: "toast.scene_markers_copy_failed:1",
    });
    expect(page.success).not.toHaveBeenCalled();
  });
});
