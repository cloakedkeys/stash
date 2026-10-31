import { createRoot } from "react-dom/client";
import {
  ApolloClient,
  ApolloLink,
  ApolloProvider,
  InMemoryCache,
  Observable,
} from "@apollo/client";
import { MemoryRouter } from "react-router-dom";
import { IntlProvider } from "react-intl";
import VoiceActors from "../src/components/VoiceActors/VoiceActors";
import messages from "../src/locales/en-GB.json";
import flattenMessages from "../src/utils/flattenMessages";

export function mount() {
  const actors = new Map<string, Record<string, unknown>>();
  const requests: unknown[] = [];
  let id = 0;
  const client = new ApolloClient({
    cache: new InMemoryCache(),
    link: new ApolloLink(
      (operation) =>
        new Observable((observer) => {
          const { operationName: name, variables: vars } = operation;
          requests.push({ name, vars });
          let data: Record<string, unknown>;
          switch (name) {
            case "FindVoiceActors": {
              const items = [...actors.values()].filter(
                (a) =>
                  (!vars.favorite || a.favorite) &&
                  (!vars.filter?.q ||
                    String(a.name).includes(vars.filter.q) ||
                    (a.aliases as string[]).some((alias) =>
                      alias.includes(vars.filter.q)
                    ))
              );
              data = {
                findVoiceActors: { count: items.length, voice_actors: items },
              };
              break;
            }
            case "FindVoiceActor":
              data = { findVoiceActor: actors.get(vars.id) ?? null };
              break;
            case "VoiceActorCreate": {
              const actor = {
                __typename: "VoiceActor",
                id: String(++id),
                created_at: "2026-10-30T00:00:00Z",
                updated_at: "2026-10-30T00:00:00Z",
                scene_count: 0,
                scenes: [],
                ...vars.input,
              };
              actors.set(actor.id, actor);
              data = { voiceActorCreate: actor };
              break;
            }
            case "VoiceActorUpdate": {
              const actor = { ...actors.get(vars.input.id), ...vars.input };
              actors.set(vars.input.id, actor);
              data = { voiceActorUpdate: actor };
              break;
            }
            case "VoiceActorDestroy":
              actors.delete(vars.id);
              data = { voiceActorDestroy: true };
              break;
            default:
              observer.error(new Error(`Unexpected operation ${name}`));
              return;
          }
          observer.next({ data });
          observer.complete();
        })
    ),
  });
  const host = document.createElement("div");
  host.id = "voice-actors-fixture";
  host.style.cssText =
    "position:absolute;inset:0;background:#202b33;padding:24px;z-index:10000;overflow:auto";
  document.body.append(host);
  createRoot(host).render(
    <ApolloProvider client={client}>
      <MemoryRouter initialEntries={["/voice-actors"]}>
        <IntlProvider locale="en-GB" messages={flattenMessages(messages)}>
          <VoiceActors />
        </IntlProvider>
      </MemoryRouter>
    </ApolloProvider>
  );
  return { actors, requests };
}
