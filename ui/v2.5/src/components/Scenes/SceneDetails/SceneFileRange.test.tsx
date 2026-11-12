import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";
import type { ComponentProps, ReactNode } from "react";
import * as GQL from "src/core/generated-graphql";
import messages from "src/locales/en-GB.json";
import { SceneCreateFromPartDialog } from "./SceneCreateFromPartDialog";
import {
  SceneFileRangeDescription,
  SceneFileRangeFields,
} from "./SceneFileRange";

const page = vi.hoisted(() => ({
  state: [] as unknown[],
  next: 0,
  buttons: [] as { onClick?: () => void; children?: ReactNode }[],
  durations: [] as {
    value: number | null | undefined;
    setValue: (value: number | null) => void;
  }[],
  accept: undefined as undefined | { onClick: () => Promise<void> },
  disabled: undefined as boolean | undefined,
  title: undefined as
    | undefined
    | {
        value: string;
        onChange: (e: { currentTarget: { value: string } }) => void;
      },
  position: vi.fn(),
  create: vi.fn(),
  push: vi.fn(),
  error: vi.fn(),
}));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useState: (initial: unknown) => {
    const index = page.next++;
    if (!(index in page.state)) page.state[index] = initial;
    return [
      page.state[index],
      (value: unknown) => {
        page.state[index] = value;
      },
    ];
  },
}));
vi.mock("react-bootstrap", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-bootstrap")>();
  return {
    ...original,
    Button: (props: (typeof page.buttons)[number]) => {
      page.buttons.push(props);
      return <button type="button">{props.children}</button>;
    },
    Form: Object.assign(original.Form, {
      Control: (props: NonNullable<typeof page.title>) => {
        page.title = props;
        return <input readOnly value={props.value} />;
      },
    }),
  };
});
vi.mock("src/components/Shared/DurationInput", () => ({
  DurationInput: (props: (typeof page.durations)[number]) => {
    page.durations.push(props);
    return null;
  },
}));
vi.mock("src/components/Shared/Modal", () => ({
  ModalComponent: (props: {
    children: ReactNode;
    accept: typeof page.accept;
    disabled?: boolean;
  }) => {
    page.accept = props.accept;
    page.disabled = props.disabled;
    return <>{props.children}</>;
  },
}));
vi.mock("react-router-dom", () => ({
  useHistory: () => ({ push: page.push }),
}));
vi.mock("src/components/ScenePlayer/util", () => ({
  getPlayerPosition: page.position,
}));
vi.mock("src/core/StashService", () => ({
  mutateCreateScene: page.create,
}));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ error: page.error }),
}));

// Flatten only the messages used here, keeping assertions on the real English text.
const rangeMessages = Object.fromEntries(
  Object.entries(messages.scene_file_range).map(([key, value]) => [
    `scene_file_range.${key}`,
    value,
  ])
);
const intlMessages = {
  ...rangeMessages,
  title: messages.title,
  "actions.create": messages.actions.create,
};

function render(element: ReactNode) {
  page.next = 0;
  page.buttons = [];
  page.durations = [];
  return renderToStaticMarkup(
    <IntlProvider locale="en-GB" messages={intlMessages}>
      {element}
    </IntlProvider>
  );
}

beforeEach(() => {
  page.state = [];
  vi.clearAllMocks();
  page.create.mockResolvedValue({ data: { sceneCreate: { id: "9" } } });
});

describe("scene file range fields", () => {
  it("sets each field from the current player time", () => {
    const start = vi.fn();
    const end = vi.fn();
    render(
      <SceneFileRangeFields
        startTime={null}
        endTime={null}
        onStartTimeChange={start}
        onEndTimeChange={end}
      />
    );
    page.position.mockReturnValueOnce(83.25).mockReturnValueOnce(296);
    page.buttons[0].onClick!();
    page.buttons[1].onClick!();
    expect(start).toHaveBeenCalledWith(83.25);
    expect(end).toHaveBeenCalledWith(296);
  });

  it.each([
    83.25,
    0,
    null,
  ])("adds the loaded part start %s, even while editing another start", (playerStartTime) => {
    const start = vi.fn();
    const end = vi.fn();
    render(
      <SceneFileRangeFields
        playerStartTime={playerStartTime}
        startTime={200}
        endTime={400}
        onStartTimeChange={start}
        onEndTimeChange={end}
      />
    );
    page.position.mockReturnValue(12.5);
    page.buttons[0].onClick!();
    page.buttons[1].onClick!();
    expect(start).toHaveBeenCalledWith((playerStartTime ?? 0) + 12.5);
    expect(end).toHaveBeenCalledWith((playerStartTime ?? 0) + 12.5);
  });

  it("leaves the field unchanged when there is no player", () => {
    const change = vi.fn();
    render(
      <SceneFileRangeFields
        startTime={20}
        endTime={null}
        onStartTimeChange={change}
        onEndTimeChange={change}
      />
    );
    page.position.mockReturnValue(undefined);
    page.buttons[0].onClick!();
    expect(change).not.toHaveBeenCalled();
  });
});

describe("create scene from part", () => {
  function dialog() {
    return (
      <SceneCreateFromPartDialog
        fileID="7"
        sceneTitle="Original"
        onClose={vi.fn()}
      />
    );
  }

  it("creates with only the selected file, title and times, then navigates", async () => {
    render(dialog());
    expect(page.title?.value).toBe("Original (part)");
    page.title!.onChange({ currentTarget: { value: "Second scene" } });
    page.durations[0].setValue(83.25);
    page.durations[1].setValue(296);
    render(dialog());
    await page.accept!.onClick();
    expect(page.create).toHaveBeenCalledWith({
      file_ids: ["7"],
      title: "Second scene",
      start_time: 83.25,
      end_time: 296,
    });
    expect(page.push).toHaveBeenCalledWith("/scenes/9");
  });

  it("creates another part using file times from a part player", async () => {
    const element = (
      <SceneCreateFromPartDialog
        fileID="7"
        sceneTitle="Part"
        playerStartTime={83.25}
        onClose={vi.fn()}
      />
    );
    render(element);
    page.position.mockReturnValueOnce(10).mockReturnValueOnce(20);
    page.buttons[0].onClick!();
    page.buttons[1].onClick!();
    render(element);
    await page.accept!.onClick();
    expect(page.create).toHaveBeenCalledWith({
      file_ids: ["7"],
      title: "Part (part)",
      start_time: 93.25,
      end_time: 103.25,
    });
  });

  it("sends null for an empty end time", async () => {
    render(dialog());
    page.durations[0].setValue(83);
    render(dialog());
    await page.accept!.onClick();
    expect(page.create).toHaveBeenCalledWith({
      file_ids: ["7"],
      title: "Original (part)",
      start_time: 83,
      end_time: null,
    });
  });

  it("shows the server refusal and keeps the dialog open", async () => {
    const error = new Error(
      "another scene already uses the whole of this file"
    );
    const close = vi.fn();
    page.create.mockRejectedValue(error);
    render(
      <SceneCreateFromPartDialog
        fileID="7"
        sceneTitle="Original"
        onClose={close}
      />
    );
    page.durations[0].setValue(83);
    await page.accept!.onClick();
    expect(page.error).toHaveBeenCalledWith(error);
    expect(page.push).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
  });
  it.each([
    { name: "both times empty", start: null, end: null, allowed: false },
    { name: "start 0 and no end", start: 0, end: null, allowed: false },
    {
      name: "start 0 and end at the file's end",
      start: 0,
      end: 600,
      allowed: false,
    },
    { name: "only a start", start: 83, end: null, allowed: true },
    {
      name: "only an end before the file's end",
      start: null,
      end: 296,
      allowed: true,
    },
    {
      name: "an end past the file's end (server refuses)",
      start: null,
      end: 700,
      allowed: true,
    },
  ])("allows Create only for a real part: $name", ({ start, end, allowed }) => {
    const element = (
      <SceneCreateFromPartDialog
        fileID="7"
        fileDuration={600}
        sceneTitle="Original"
        onClose={vi.fn()}
      />
    );
    render(element);
    page.durations[0].setValue(start);
    page.durations[1].setValue(end);
    render(element);
    expect(page.disabled).toBe(!allowed);
  });
});

describe("scene file range description", () => {
  const file = { path: "/library/movie.mp4" } as GQL.VideoFileDataFragment;

  function description(start_time: number | null, end_time: number | null) {
    const scene: ComponentProps<typeof SceneFileRangeDescription>["scene"] = {
      files: [file],
      start_time,
      end_time,
    };
    return render(<SceneFileRangeDescription scene={scene} />);
  }

  it("shows the file name and both times", () => {
    expect(description(83, 296)).toContain(
      "Part of movie.mp4, from 1:23 to 4:56"
    );
  });

  it("shows an open end", () => {
    expect(description(83, null)).toContain(
      "Part of movie.mp4, from 1:23 to the end"
    );
  });

  it("treats an unset start as zero", () => {
    expect(description(null, 296)).toContain(
      "Part of movie.mp4, from 0:00 to 4:56"
    );
  });

  it("shows nothing for a whole-file scene", () => {
    expect(description(null, null)).toBe("");
  });
});
