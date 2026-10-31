// Production security controls with local Apollo responses; no backend required.
import { createRoot } from "react-dom/client";
import {
  ApolloClient,
  ApolloLink,
  ApolloProvider,
  InMemoryCache,
  Observable,
} from "@apollo/client";
import { IntlProvider } from "react-intl";
import { BrowserRouter } from "react-router-dom";
import { SettingsSecurityPanel } from "../src/components/Settings/SettingsSecurityPanel";
import { OrganizedButton } from "../src/components/Scenes/SceneDetails/OrganizedButton";
import { CurrentUserProvider } from "../src/hooks/CurrentUser";
import { ToastProvider } from "../src/hooks/Toast";
import {
  SettingStateContext,
  ISettingsContextState,
} from "../src/components/Settings/context";
import { RoleEnum } from "../src/core/generated-graphql";
import english from "../src/locales/en-GB.json";
import "../src/index.scss";

function flatten(
  source: Record<string, unknown>,
  prefix = ""
): Record<string, string> {
  return Object.assign(
    {},
    ...Object.entries(source).map(([key, value]) =>
      typeof value === "string"
        ? { [prefix + key]: value }
        : flatten(value as Record<string, unknown>, prefix + key + ".")
    )
  );
}
const messages = flatten(english);
let root: ReturnType<typeof createRoot> | undefined;
export const events: { operation: string; variables: unknown }[] = [];
export function renderCase(role: RoleEnum = RoleEnum.Admin, guest = false) {
  root?.unmount();
  const host = document.getElementById("fixture")!;
  root = createRoot(host);
  events.length = 0;
  const client = new ApolloClient({
    cache: new InMemoryCache(),
    link: new ApolloLink(
      (operation) =>
        new Observable((observer) => {
          events.push({
            operation: operation.operationName,
            variables: structuredClone(operation.variables),
          });
          observer.next({
            data:
              operation.operationName === "GenerateAPIKey"
                ? {
                    generateAPIKey: operation.variables.input.clear
                      ? ""
                      : "fixture-user-api-key",
                  }
                : { changePassword: true },
          });
          observer.complete();
        })
    ),
  });
  const state = {
    advancedMode: false,
    loading: false,
    general: { username: null, password: null, maxSessionAge: 3600 },
    saveGeneral: (variables: unknown) =>
      events.push({ operation: "saveGeneral", variables }),
  } as ISettingsContextState;
  root.render(
    <BrowserRouter>
      <ApolloProvider client={client}>
        <IntlProvider locale="en" messages={messages}>
          <ToastProvider>
            <CurrentUserProvider
              user={{ name: guest ? "guest" : "fixture-user", roles: [role] }}
            >
              <SettingStateContext.Provider value={state}>
                <main className="container py-3">
                  <h2>Multi-user security fixture</h2>
                  <SettingsSecurityPanel />
                  <p>
                    Scene organized status:{" "}
                    <OrganizedButton
                      loading={false}
                      organized={false}
                      onClick={() =>
                        events.push({ operation: "organized", variables: {} })
                      }
                    />
                  </p>
                </main>
              </SettingStateContext.Provider>
            </CurrentUserProvider>
          </ToastProvider>
        </IntlProvider>
      </ApolloProvider>
    </BrowserRouter>
  );
}
Object.assign(window, { multiUserHarness: { renderCase, events } });
renderCase();
