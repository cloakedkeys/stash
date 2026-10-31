import React, { useRef, useState } from "react";
import { ModalSetting, NumberSetting } from "./Inputs";
import { SettingSection } from "./SettingSection";
import * as GQL from "src/core/generated-graphql";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { useSettings } from "./context";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import { useCurrentUser, useIsAdmin } from "src/hooks/CurrentUser";
import { LibraryLimits } from "./LibraryLimits";

const PasswordInput: React.FC<{
  value: GQL.UserChangePasswordInput;
  setValue: (value: GQL.UserChangePasswordInput) => void;
}> = ({ value, setValue }) => (
  <>
    <Form.Group controlId="existing-password">
      <Form.Label>
        <FormattedMessage
          id="config.general.auth.current_password"
          defaultMessage="Current password"
        />
      </Form.Label>
      <Form.Control
        type="password"
        autoComplete="current-password"
        value={value.existingPassword}
        onChange={(event) =>
          setValue({ ...value, existingPassword: event.currentTarget.value })
        }
      />
    </Form.Group>
    <Form.Group controlId="new-password">
      <Form.Label>
        <FormattedMessage
          id="config.general.auth.new_password"
          defaultMessage="New password"
        />
      </Form.Label>
      <Form.Control
        type="password"
        autoComplete="new-password"
        value={value.newPassword}
        onChange={(event) =>
          setValue({ ...value, newPassword: event.currentTarget.value })
        }
      />
    </Form.Group>
  </>
);

export const SettingsSecurityPanel: React.FC = () => {
  const intl = useIntl();
  const Toast = useToast();
  const user = useCurrentUser();
  const isAdmin = useIsAdmin();
  const { general, loading, error, saveGeneral } = useSettings();
  const [apiKey, setAPIKey] = useState("");
  const passwordChangePending = useRef(false);
  const [generateAPIKey, { loading: generating }] =
    GQL.useGenerateApiKeyMutation();
  const [changePassword, { loading: changingPassword }] =
    GQL.useChangePasswordMutation();
  const canGenerateAPIKey =
    isAdmin || user?.roles?.includes(GQL.RoleEnum.GenerateApiKey);

  async function onChangePassword(input: GQL.UserChangePasswordInput) {
    if (passwordChangePending.current) return;
    passwordChangePending.current = true;
    try {
      await changePassword({ variables: { input } });
      Toast.success(
        intl.formatMessage({
          id: "config.general.auth.password_changed",
          defaultMessage:
            "Password changed. Sign in again with your new password.",
        })
      );
    } catch (error) {
      Toast.error(error);
    } finally {
      passwordChangePending.current = false;
    }
  }

  async function onGenerateAPIKey(clear: boolean) {
    try {
      const result = await generateAPIKey({ variables: { input: { clear } } });
      setAPIKey(clear ? "" : (result.data?.generateAPIKey ?? ""));
    } catch (error) {
      Toast.error(error);
    }
  }

  if (error) return <h1>{error.message}</h1>;
  if (loading) return <LoadingIndicator />;

  return (
    <>
      <SettingSection headingID="config.general.auth.authentication">
        <ModalSetting<GQL.UserChangePasswordInput>
          id="authentication-settings"
          heading={intl.formatMessage({
            id: "config.general.auth.change_password",
            defaultMessage: "Change password",
          })}
          subHeading={intl.formatMessage({
            id: "config.general.auth.change_password_desc",
            defaultMessage: "Change the password for your account.",
          })}
          value={{ existingPassword: "", newPassword: "" }}
          disabled={!user || user.name === "guest" || changingPassword}
          onChange={onChangePassword}
          renderField={(value, setValue) => (
            <PasswordInput
              value={value ?? { existingPassword: "", newPassword: "" }}
              setValue={setValue}
            />
          )}
          renderValue={() => user?.name}
        />
        <div className="setting" id="apikey">
          <div>
            <h3>{intl.formatMessage({ id: "config.general.auth.api_key" })}</h3>
            {apiKey && (
              <Form.Control
                as="textarea"
                aria-label={intl.formatMessage({
                  id: "config.general.auth.api_key",
                })}
                readOnly
                rows={3}
                value={apiKey}
              />
            )}
            <div className="sub-heading">
              <FormattedMessage
                id="config.general.auth.api_key_once"
                defaultMessage="Your API key is only shown when generated. Copy it before leaving this page."
              />
            </div>
          </div>
          <div>
            <Button
              disabled={!canGenerateAPIKey || generating}
              onClick={() => onGenerateAPIKey(false)}
            >
              {intl.formatMessage({
                id: "config.general.auth.generate_api_key",
              })}
            </Button>
            <Button
              variant="danger"
              disabled={!canGenerateAPIKey || generating}
              onClick={() => onGenerateAPIKey(true)}
            >
              {intl.formatMessage({ id: "config.general.auth.clear_api_key" })}
            </Button>
          </div>
        </div>
        <NumberSetting
          id="maxSessionAge"
          headingID="config.general.auth.maximum_session_age"
          subHeadingID="config.general.auth.maximum_session_age_desc"
          value={general.maxSessionAge ?? undefined}
          disabled={!isAdmin}
          onChange={(value) => saveGeneral({ maxSessionAge: value })}
        />
      </SettingSection>
      {isAdmin && <LibraryLimits />}
    </>
  );
};
