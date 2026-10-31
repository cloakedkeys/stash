import React, { useState } from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { useCurrentUser } from "src/hooks/CurrentUser";
import { useToast } from "src/hooks/Toast";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { ModalComponent } from "../Shared/Modal";
import { SettingSection } from "./SettingSection";

type Account = GQL.FindUsersForAccountsQuery["users"][number];

type AccessLevel = "admin" | "modify" | "read";

const refetchQueries = ["FindUsersForAccounts", "FindUsersForLibraryLimits"];

function accessLevel(roles: GQL.RoleEnum[]): AccessLevel {
  if (roles.includes(GQL.RoleEnum.Admin)) return "admin";
  if (roles.includes(GQL.RoleEnum.Modify)) return "modify";
  return "read";
}

function rolesFor(level: AccessLevel, apiKey: boolean) {
  const roles =
    level === "admin"
      ? [GQL.RoleEnum.Admin]
      : level === "modify"
        ? [GQL.RoleEnum.Read, GQL.RoleEnum.Modify]
        : [GQL.RoleEnum.Read];
  if (apiKey && level !== "admin") {
    roles.push(GQL.RoleEnum.GenerateApiKey);
  }
  return roles;
}

const AccountDialog: React.FC<{
  account?: Account;
  onClose: () => void;
}> = ({ account, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [createUser, { loading: creating }] = GQL.useUserCreateMutation({
    refetchQueries,
  });
  const [updateUser, { loading: updating }] = GQL.useUserUpdateMutation({
    refetchQueries,
  });

  const existingRoles = account?.roles ?? [GQL.RoleEnum.Read];
  const [name, setName] = useState(account?.name ?? "");
  const [notes, setNotes] = useState(account?.notes ?? "");
  const [password, setPassword] = useState("");
  const [level, setLevel] = useState(accessLevel(existingRoles));
  const [apiKey, setApiKey] = useState(
    existingRoles.includes(GQL.RoleEnum.GenerateApiKey)
  );

  async function save() {
    const roles = rolesFor(level, apiKey);
    try {
      if (account) {
        await updateUser({
          variables: {
            input: { existingName: account.name, name, notes, roles },
          },
        });
      } else {
        await createUser({
          variables: { input: { name, notes, password, roles } },
        });
      }
      onClose();
    } catch (error) {
      Toast.error(error);
    }
  }

  return (
    <ModalComponent
      show
      header={intl.formatMessage({
        id: account
          ? "config.general.auth.accounts.edit"
          : "config.general.auth.accounts.add",
      })}
      isRunning={creating || updating}
      disabled={!name || (!account && !password)}
      accept={{
        text: intl.formatMessage({ id: "actions.save" }),
        onClick: save,
      }}
      cancel={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: onClose,
      }}
    >
      <Form.Group controlId="account-name">
        <Form.Label>
          <FormattedMessage id="config.general.auth.username" />
        </Form.Label>
        <Form.Control
          className="text-input"
          autoComplete="off"
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
        />
      </Form.Group>
      {!account && (
        <Form.Group controlId="account-password">
          <Form.Label>
            <FormattedMessage id="config.general.auth.password" />
          </Form.Label>
          <Form.Control
            className="text-input"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
          />
        </Form.Group>
      )}
      <Form.Group controlId="account-level">
        <Form.Label>
          <FormattedMessage id="config.general.auth.accounts.access" />
        </Form.Label>
        <Form.Control
          as="select"
          className="input-control"
          value={level}
          onChange={(e) => setLevel(e.currentTarget.value as AccessLevel)}
        >
          {(["read", "modify", "admin"] as const).map((l) => (
            <option key={l} value={l}>
              {intl.formatMessage({
                id: `config.general.auth.accounts.access_${l}`,
              })}
            </option>
          ))}
        </Form.Control>
      </Form.Group>
      {level !== "admin" && (
        <Form.Group>
          <Form.Check
            id="account-api-key"
            checked={apiKey}
            onChange={() => setApiKey(!apiKey)}
            label={intl.formatMessage({
              id: "config.general.auth.accounts.can_generate_api_key",
            })}
          />
        </Form.Group>
      )}
      <Form.Group controlId="account-notes">
        <Form.Label>
          <FormattedMessage id="config.general.auth.accounts.notes" />
        </Form.Label>
        <Form.Control
          as="textarea"
          className="text-input"
          value={notes}
          onChange={(e) => setNotes(e.currentTarget.value)}
        />
      </Form.Group>
    </ModalComponent>
  );
};

const ResetPasswordDialog: React.FC<{
  account: Account;
  onClose: () => void;
}> = ({ account, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [changePassword, { loading }] = GQL.useChangeUserPasswordMutation();
  const [password, setPassword] = useState("");

  async function save() {
    try {
      await changePassword({
        variables: { input: { name: account.name, newPassword: password } },
      });
      Toast.success(
        intl.formatMessage({ id: "config.general.auth.accounts.password_set" })
      );
      onClose();
    } catch (error) {
      Toast.error(error);
    }
  }

  return (
    <ModalComponent
      show
      header={account.name}
      isRunning={loading}
      disabled={!password}
      accept={{
        text: intl.formatMessage({ id: "actions.save" }),
        onClick: save,
      }}
      cancel={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: onClose,
      }}
    >
      <Form.Group controlId="account-new-password">
        <Form.Label>
          <FormattedMessage id="config.general.auth.new_password" />
        </Form.Label>
        <Form.Control
          className="text-input"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
        />
      </Form.Group>
    </ModalComponent>
  );
};

const DeleteDialog: React.FC<{
  account: Account;
  onClose: () => void;
}> = ({ account, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [destroyUser, { loading }] = GQL.useUserDestroyMutation({
    refetchQueries,
  });

  async function confirm() {
    try {
      await destroyUser({ variables: { input: { name: account.name } } });
      onClose();
    } catch (error) {
      Toast.error(error);
    }
  }

  return (
    <ModalComponent
      show
      header={account.name}
      isRunning={loading}
      accept={{
        text: intl.formatMessage({ id: "actions.delete" }),
        variant: "danger",
        onClick: confirm,
      }}
      cancel={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: onClose,
      }}
    >
      <FormattedMessage
        id="config.general.auth.accounts.delete_confirm"
        values={{ name: account.name }}
      />
    </ModalComponent>
  );
};

export const Accounts: React.FC = () => {
  const intl = useIntl();
  const Toast = useToast();
  const currentUser = useCurrentUser();
  const { data, loading } = GQL.useFindUsersForAccountsQuery();
  const [lockUser] = GQL.useLockUserMutation({ refetchQueries });
  const [unlockUser] = GQL.useUnlockUserMutation({ refetchQueries });

  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Account>();
  const [resetting, setResetting] = useState<Account>();
  const [deleting, setDeleting] = useState<Account>();

  async function toggleLock(account: Account) {
    const variables = { name: account.name };
    try {
      if (account.locked) {
        await unlockUser({ variables });
      } else {
        await lockUser({ variables });
      }
    } catch (error) {
      Toast.error(error);
    }
  }

  function describe(account: Account) {
    const parts = [
      intl.formatMessage({
        id: `config.general.auth.accounts.access_${accessLevel(account.roles ?? [])}`,
      }),
    ];
    if (account.locked) {
      parts.push(
        intl.formatMessage({ id: "config.general.auth.accounts.locked" })
      );
    }
    if (account.notes) {
      parts.push(account.notes);
    }
    return parts.join(" · ");
  }

  return (
    <SettingSection
      id="accounts"
      headingID="config.general.auth.accounts.heading"
      subHeadingID="config.general.auth.accounts.description"
    >
      {loading && <LoadingIndicator inline small message="" />}
      {data?.users.map((account) => {
        const isCurrent = account.name === currentUser?.name;
        return (
          <div className="setting" key={account.name}>
            <div>
              <h3>{account.name}</h3>
              <div className="sub-heading">{describe(account)}</div>
            </div>
            <div>
              <Button onClick={() => setEditing(account)}>
                <FormattedMessage id="actions.edit" />
              </Button>
              <Button onClick={() => setResetting(account)}>
                <FormattedMessage id="config.general.auth.accounts.reset_password" />
              </Button>
              <Button disabled={isCurrent} onClick={() => toggleLock(account)}>
                <FormattedMessage
                  id={
                    account.locked
                      ? "config.general.auth.accounts.unlock"
                      : "config.general.auth.accounts.lock"
                  }
                />
              </Button>
              <Button
                variant="danger"
                disabled={isCurrent}
                onClick={() => setDeleting(account)}
              >
                <FormattedMessage id="actions.delete" />
              </Button>
            </div>
          </div>
        );
      })}
      <div className="setting">
        <div />
        <div>
          <Button onClick={() => setAdding(true)}>
            <FormattedMessage id="config.general.auth.accounts.add" />
          </Button>
        </div>
      </div>
      {adding && <AccountDialog onClose={() => setAdding(false)} />}
      {editing && (
        <AccountDialog
          account={editing}
          onClose={() => setEditing(undefined)}
        />
      )}
      {resetting && (
        <ResetPasswordDialog
          account={resetting}
          onClose={() => setResetting(undefined)}
        />
      )}
      {deleting && (
        <DeleteDialog
          account={deleting}
          onClose={() => setDeleting(undefined)}
        />
      )}
    </SettingSection>
  );
};
