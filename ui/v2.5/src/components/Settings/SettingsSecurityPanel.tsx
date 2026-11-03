import React, { useRef, useState } from "react";
import { ModalSetting } from "./Inputs";
import { SettingSection } from "./SettingSection";
import * as GQL from "src/core/generated-graphql";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { useSettings } from "./context";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import { useCurrentUser, useIsAdmin } from "src/hooks/CurrentUser";
import { Accounts } from "./Accounts";
import { LibraryLimits } from "./LibraryLimits";

const maxSessionAge = 400 * 24 * 60 * 60;
const sessionAgeChoices = [
  3600,
  86400,
  604800,
  2592000,
  7776000,
  maxSessionAge,
];

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

  function formatSessionAge(value: number) {
    if (value > 0 && value % 86400 === 0) {
      return intl.formatMessage(
        {
          id: "config.general.auth.session_age_days",
          defaultMessage: "{count, plural, one {# day} other {# days}}",
        },
        { count: value / 86400 }
      );
    }
    if (value > 0 && value % 3600 === 0) {
      return intl.formatMessage(
        {
          id: "config.general.auth.session_age_hours",
          defaultMessage: "{count, plural, one {# hour} other {# hours}}",
        },
        { count: value / 3600 }
      );
    }
    return intl.formatMessage(
      {
        id: "config.general.auth.session_age_seconds",
        defaultMessage: "{count, plural, one {# second} other {# seconds}}",
      },
      { count: value }
    );
  }

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
        <ModalSetting<number>
          id="maxSessionAge"
          headingID="config.general.auth.maximum_session_age"
          subHeadingID="config.general.auth.maximum_session_age_desc"
          value={Math.min(general.maxSessionAge ?? 3600, maxSessionAge)}
          disabled={!isAdmin}
          onChange={(value) =>
            saveGeneral({ maxSessionAge: Math.min(value, maxSessionAge) })
          }
          renderField={(value, setValue) => (
            <Form.Control
              as="select"
              className="input-control"
              aria-label={intl.formatMessage({
                id: "config.general.auth.maximum_session_age",
              })}
              value={value ?? 3600}
              onChange={(event) => setValue(Number(event.currentTarget.value))}
            >
              {value !== undefined && !sessionAgeChoices.includes(value) && (
                <option value={value}>{formatSessionAge(value)}</option>
              )}
              {sessionAgeChoices.map((age) => (
                <option key={age} value={age}>
                  {formatSessionAge(age)}
                </option>
              ))}
            </Form.Control>
          )}
          renderValue={(value) => (
            <span>{formatSessionAge(value ?? 3600)}</span>
          )}
        />
      </SettingSection>
      {isAdmin && <Accounts />}
      {isAdmin && <LibraryLimits />}
    </>
  );
};
