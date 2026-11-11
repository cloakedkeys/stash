import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useIsAdmin } from "src/hooks/CurrentUser";
import { EditTagsDialog } from "./EditTagsDialog";

vi.mock("src/core/StashService", () => ({
  useBulkTagUpdate: vi.fn(() => [vi.fn()]),
}));

vi.mock("src/hooks/CurrentUser", () => ({ useIsAdmin: vi.fn() }));
vi.mock("src/hooks/Toast", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));
// Render modal contents without a DOM portal. Keep the real form and checkbox.
vi.mock("../Shared/Modal", () => ({
  ModalComponent: ({ children }: React.PropsWithChildren) => (
    <div>{children}</div>
  ),
}));
vi.mock("../Shared/MultiSet", () => ({ MultiSet: () => null }));
vi.mock("../Shared/BulkUpdate", () => ({
  BulkUpdateFormGroup: ({ children }: React.PropsWithChildren) => (
    <div>{children}</div>
  ),
  BulkUpdateTextInput: () => null,
}));

beforeEach(() => {
  vi.clearAllMocks();
  // The dialogs pass document.body to the mocked MultiSet portal.
  vi.stubGlobal("document", { body: {} });
});

afterEach(() => vi.unstubAllGlobals());

function renderDialog() {
  return renderToStaticMarkup(
    <IntlProvider
      locale="en"
      messages={{ organized: "Organized", favourite: "Favourite" }}
      onError={() => {}}
    >
      <EditTagsDialog selected={[]} onClose={() => {}} />
    </IntlProvider>
  );
}

it("shows Organized for admins and hides it for MODIFY users", () => {
  vi.mocked(useIsAdmin).mockReturnValue(true);
  const adminMarkup = renderDialog();
  const checkbox = adminMarkup
    .match(/<input\b[^>]*>/g)
    ?.find((input) => input.includes('id="organized"'));

  expect(checkbox).toContain('type="checkbox"');
  expect(adminMarkup).toContain('for="organized"');
  expect(adminMarkup).toContain(">Organized</label>");

  // MODIFY users can edit entities, but cannot change Organized.
  vi.mocked(useIsAdmin).mockReturnValue(false);
  const modifyMarkup = renderDialog();

  expect(modifyMarkup).toContain(">Favourite</label>");
  expect(modifyMarkup).not.toContain('id="organized"');
  expect(modifyMarkup).not.toContain('for="organized"');
  expect(modifyMarkup).not.toContain("Organized");
});
