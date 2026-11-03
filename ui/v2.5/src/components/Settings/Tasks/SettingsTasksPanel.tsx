import React, { useEffect, useRef, useState } from "react";
import { useHistory, useLocation } from "react-router-dom";
import { useIntl } from "react-intl";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { LibraryTasks } from "./LibraryTasks";
import { DataManagementTasks } from "./DataManagementTasks";
import { PluginTasks } from "./PluginTasks";
import { JobTable } from "./JobTable";

export const SettingsTasksPanel: React.FC = () => {
  const intl = useIntl();
  const history = useHistory();
  const location = useLocation();
  const queueRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Tab navigation keeps the previous page position. Bring the queue into
    // view, but let Back/Forward restore its saved scroll position.
    if (
      history.action === "PUSH" &&
      new URLSearchParams(location.search).get("tab") === "tasks"
    ) {
      // Wait for the tab pane to become visible before measuring its position.
      const frame = requestAnimationFrame(() => {
        queueRef.current?.scrollIntoView({ block: "start" });
      });
      return () => cancelAnimationFrame(frame);
    }
  }, [history.action, location.search]);

  const [isBackupRunning, setIsBackupRunning] = useState<boolean>(false);
  const [isAnonymiseRunning, setIsAnonymiseRunning] = useState<boolean>(false);

  if (isBackupRunning) {
    return (
      <LoadingIndicator
        message={intl.formatMessage({ id: "config.tasks.backing_up_database" })}
      />
    );
  }

  if (isAnonymiseRunning) {
    return (
      <LoadingIndicator
        message={intl.formatMessage({
          id: "config.tasks.anonymising_database",
        })}
      />
    );
  }

  return (
    <div id="tasks-panel">
      <div className="tasks-panel-queue" ref={queueRef}>
        <h1>{intl.formatMessage({ id: "config.tasks.job_queue" })}</h1>
        <JobTable />
      </div>

      <div className="tasks-panel-tasks">
        <LibraryTasks />
        <hr />
        <DataManagementTasks
          setIsBackupRunning={setIsBackupRunning}
          setIsAnonymiseRunning={setIsAnonymiseRunning}
        />
        <hr />
        <PluginTasks />
      </div>
    </div>
  );
};
