import { createRoot } from "react-dom/client";
import {
  ApolloClient,
  ApolloLink,
  ApolloProvider,
  HttpLink,
  InMemoryCache,
  Observable,
} from "@apollo/client";
import { MemoryRouter } from "react-router-dom";
import { IntlProvider } from "react-intl";
import { ConfigurationProvider } from "../src/hooks/Config";
import { ToastProvider } from "../src/hooks/Toast";
import { StudioEditPanel } from "../src/components/Studios/StudioDetails/StudioEditPanel";
import messages from "../src/locales/en-GB.json";
import flattenMessages from "../src/utils/flattenMessages";
import * as GQL from "../src/core/generated-graphql";

const image =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl6P2AAAAAASUVORK5CYII=";
const scraped = {
  __typename: "ScrapedStudio",
  name: "Scraped Studio",
  details: "Scraped details",
  aliases: "Remote alias",
  urls: ["https://studio.example"],
  image,
  remote_site_id: "remote-studio",
  parent: null,
  tags: [],
};

export function mount(configuration: GQL.ConfigDataFragment, isNew = false) {
  const requests: unknown[] = [];
  const results: unknown[] = [];
  const client = new ApolloClient({
    cache: new InMemoryCache(),
    link: ApolloLink.split(
      (operation) => operation.operationName === "ScrapeSingleStudio",
      new ApolloLink(
        (operation) =>
          new Observable((observer) => {
            requests.push(operation.variables);
            observer.next({ data: { scrapeSingleStudio: [scraped] } });
            observer.complete();
          })
      ),
      new HttpLink({ uri: "/graphql" })
    ),
  });
  const host = document.createElement("div");
  host.id = "studio-scraping-fixture";
  host.style.cssText =
    "position:absolute;inset:0;background:#202b33;padding:24px;z-index:10000;overflow:auto";
  document.body.append(host);
  const config = {
    ...configuration,
    general: {
      ...configuration.general,
      stashBoxes: [
        {
          name: "Fixture StashBox",
          endpoint: "https://stashbox.example/graphql",
          api_key: "fixture",
        },
      ],
    },
  };
  createRoot(host).render(
    <ApolloProvider client={client}>
      <MemoryRouter>
        <IntlProvider locale="en-GB" messages={flattenMessages(messages)}>
          <ConfigurationProvider configuration={config}>
            <ToastProvider>
              <StudioEditPanel
                studio={{
                  ...(isNew ? {} : { id: "fixture" }),
                  name: "Original Studio",
                  aliases: [],
                  urls: [],
                  tags: [],
                  stash_ids: [],
                  child_studios: [],
                  custom_fields: {},
                }}
                onSubmit={async (input) => {
                  results.push(input);
                }}
                onCancel={() => results.push("cancel")}
                onDelete={() => results.push("delete")}
                setImage={(value) => {
                  host.dataset.image = value ?? "";
                }}
                setEncodingImage={() => {}}
              />
            </ToastProvider>
          </ConfigurationProvider>
        </IntlProvider>
      </MemoryRouter>
    </ApolloProvider>
  );
  return { requests, results, image };
}
