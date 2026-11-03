import React, { PropsWithChildren } from "react";
import { Card } from "react-bootstrap";
import { useIntl } from "react-intl";
import { useSettings } from "./context";
import { SettingDescription } from "./SettingDescription";

interface ISettingGroup {
  id?: string;
  headingID?: string;
  subHeadingID?: string;
  advanced?: boolean;
}

export const SettingSection: React.FC<PropsWithChildren<ISettingGroup>> = ({
  id,
  children,
  headingID,
  subHeadingID,
  advanced,
}) => {
  const intl = useIntl();
  const { advancedMode } = useSettings();

  if (advanced && !advancedMode) return null;

  return (
    <div className="setting-section" id={id}>
      <h1>{headingID ? intl.formatMessage({ id: headingID }) : undefined}</h1>
      {subHeadingID ? (
        <div className="sub-heading">
          <SettingDescription>
            {intl.formatMessage({ id: subHeadingID })}
          </SettingDescription>
        </div>
      ) : undefined}
      <Card>{children}</Card>
    </div>
  );
};
