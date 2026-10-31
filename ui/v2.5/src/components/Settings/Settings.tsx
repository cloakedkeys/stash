import React from "react";
import { Tab, Nav, Row, Col, Form } from "react-bootstrap";
import { Redirect, useLocation } from "react-router-dom";
import { LinkContainer } from "react-router-bootstrap";
import { FormattedMessage } from "react-intl";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import { SettingsAboutPanel } from "./SettingsAboutPanel";
import { SettingsConfigurationPanel } from "./SettingsSystemPanel";
import { SettingsInterfacePanel } from "./SettingsInterfacePanel/SettingsInterfacePanel";
import { SettingsLogsPanel } from "./SettingsLogsPanel";
import { SettingsTasksPanel } from "./Tasks/SettingsTasksPanel";
import { SettingsPluginsPanel } from "./SettingsPluginsPanel";
import { SettingsScrapingPanel } from "./SettingsScrapingPanel";
import { SettingsToolsPanel } from "./SettingsToolsPanel";
import { SettingsServicesPanel } from "./SettingsServicesPanel";
import { SettingsContext, useSettings } from "./context";
import { SettingsLibraryPanel } from "./SettingsLibraryPanel";
import { SettingsSecurityPanel } from "./SettingsSecurityPanel";
import Changelog from "../Changelog/Changelog";
import { TroubleshootingModeButton } from "../TroubleshootingMode/TroubleshootingModeButton";
import { useTroubleshootingMode } from "../TroubleshootingMode/useTroubleshootingMode";
import { useCanAdminister, useCanModify } from "src/hooks/CurrentUser";

const validTabs = [
  "tasks",
  "library",
  "interface",
  "security",
  "metadata-providers",
  "services",
  "system",
  "plugins",
  "logs",
  "tools",
  "changelog",
  "about",
] as const;
type TabKey = (typeof validTabs)[number];

// What an account without the administrator role can use. The other tabs only show or
// change what is limited to administrators.
const limitedTabs: readonly TabKey[] = [
  "interface",
  "security",
  "changelog",
  "about",
];

function useAllowedTabs(): readonly TabKey[] {
  const canAdminister = useCanAdminister();
  const canModify = useCanModify();

  if (canAdminister) return validTabs;
  if (canModify) return [...limitedTabs, "tools"];
  return limitedTabs;
}

function isTabKey(tab: string | null): tab is TabKey {
  return validTabs.includes(tab as TabKey);
}

const SettingTabs: React.FC<{ tab: TabKey; tabs: readonly TabKey[] }> = ({
  tab,
  tabs,
}) => {
  const canAdminister = useCanAdminister();
  const { advancedMode, setAdvancedMode } = useSettings();
  const { isActive: troubleshootingModeActive } = useTroubleshootingMode();

  const titleProps = useTitleProps({ id: "settings" });

  return (
    <Tab.Container activeKey={tab} id="configuration-tabs">
      <Helmet {...titleProps} />
      <Row>
        <Col id="settings-menu-container" sm={3} md={3} xl={2}>
          <Nav variant="pills" className="flex-column">
            {tabs.includes("tasks") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=tasks">
                  <Nav.Link eventKey="tasks">
                    <FormattedMessage id="config.categories.tasks" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("library") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=library">
                  <Nav.Link eventKey="library">
                    <FormattedMessage id="library" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("interface") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=interface">
                  <Nav.Link eventKey="interface">
                    <FormattedMessage id="config.categories.interface" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("security") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=security">
                  <Nav.Link eventKey="security">
                    <FormattedMessage id="config.categories.security" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("metadata-providers") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=metadata-providers">
                  <Nav.Link eventKey="metadata-providers">
                    <FormattedMessage id="config.categories.metadata_providers" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("services") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=services">
                  <Nav.Link eventKey="services">
                    <FormattedMessage id="config.categories.services" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("system") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=system">
                  <Nav.Link eventKey="system">
                    <FormattedMessage id="config.categories.system" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("plugins") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=plugins">
                  <Nav.Link eventKey="plugins">
                    <FormattedMessage id="config.categories.plugins" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("logs") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=logs">
                  <Nav.Link eventKey="logs">
                    <FormattedMessage id="config.categories.logs" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("tools") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=tools">
                  <Nav.Link eventKey="tools">
                    <FormattedMessage id="config.categories.tools" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("changelog") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=changelog">
                  <Nav.Link eventKey="changelog">
                    <FormattedMessage id="config.categories.changelog" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            {tabs.includes("about") && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=about">
                  <Nav.Link eventKey="about">
                    <FormattedMessage id="config.categories.about" />
                  </Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            <Nav.Item>
              <div className="advanced-switch">
                <Form.Label htmlFor="advanced-settings">
                  <FormattedMessage id="config.advanced_mode" />
                </Form.Label>
                <Form.Switch
                  id="advanced-settings"
                  checked={advancedMode}
                  onChange={() => setAdvancedMode(!advancedMode)}
                />
              </div>
            </Nav.Item>
            {canAdminister && !troubleshootingModeActive && (
              <TroubleshootingModeButton />
            )}
            <hr className="d-sm-none" />
          </Nav>
        </Col>
        <Col
          id="settings-container"
          sm={{ offset: 3 }}
          md={{ offset: 3 }}
          xl={{ offset: 2 }}
        >
          <Tab.Content className="mx-auto">
            {tabs.includes("library") && (
              <Tab.Pane eventKey="library">
                <SettingsLibraryPanel />
              </Tab.Pane>
            )}
            {tabs.includes("interface") && (
              <Tab.Pane eventKey="interface">
                <SettingsInterfacePanel />
              </Tab.Pane>
            )}
            {tabs.includes("security") && (
              <Tab.Pane eventKey="security">
                <SettingsSecurityPanel />
              </Tab.Pane>
            )}
            {tabs.includes("tasks") && (
              <Tab.Pane eventKey="tasks">
                <SettingsTasksPanel />
              </Tab.Pane>
            )}
            {tabs.includes("services") && (
              <Tab.Pane eventKey="services" unmountOnExit>
                <SettingsServicesPanel />
              </Tab.Pane>
            )}
            {tabs.includes("tools") && (
              <Tab.Pane eventKey="tools" unmountOnExit>
                <SettingsToolsPanel />
              </Tab.Pane>
            )}
            {tabs.includes("metadata-providers") && (
              <Tab.Pane eventKey="metadata-providers" unmountOnExit>
                <SettingsScrapingPanel />
              </Tab.Pane>
            )}
            {tabs.includes("system") && (
              <Tab.Pane eventKey="system">
                <SettingsConfigurationPanel />
              </Tab.Pane>
            )}
            {tabs.includes("plugins") && (
              <Tab.Pane eventKey="plugins" unmountOnExit>
                <SettingsPluginsPanel />
              </Tab.Pane>
            )}
            {tabs.includes("logs") && (
              <Tab.Pane eventKey="logs" unmountOnExit>
                <SettingsLogsPanel />
              </Tab.Pane>
            )}
            {tabs.includes("changelog") && (
              <Tab.Pane eventKey="changelog" unmountOnExit>
                <Changelog />
              </Tab.Pane>
            )}
            {tabs.includes("about") && (
              <Tab.Pane eventKey="about" unmountOnExit>
                <SettingsAboutPanel />
              </Tab.Pane>
            )}
          </Tab.Content>
        </Col>
      </Row>
    </Tab.Container>
  );
};

export const Settings: React.FC = () => {
  const location = useLocation();
  const tab = new URLSearchParams(location.search).get("tab");
  const tabs = useAllowedTabs();

  if (!isTabKey(tab) || !tabs.includes(tab)) {
    return (
      <Redirect
        to={{
          ...location,
          // the first tab is the default
          search: `tab=${tabs[0]}`,
        }}
      />
    );
  }

  return (
    <SettingsContext>
      <SettingTabs tab={tab} tabs={tabs} />
    </SettingsContext>
  );
};

export default Settings;
