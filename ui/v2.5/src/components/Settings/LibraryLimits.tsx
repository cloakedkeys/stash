import React, { useMemo, useState } from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import Select from "react-select";
import * as GQL from "src/core/generated-graphql";
import { TagIDSelect } from "src/components/Tags/TagSelect";
import { StudioIDSelect } from "src/components/Studios/StudioSelect";
import { ModalComponent } from "../Shared/Modal";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import { SettingSection } from "./SettingSection";

type Account = GQL.FindUsersForLibraryLimitsQuery["users"][number];

const FolderIDSelect: React.FC<{
  ids: string[];
  onChange: (ids: string[]) => void;
}> = ({ ids, onChange }) => {
  const [query, setQuery] = useState("");

  const { data: found } = GQL.useFindFoldersForQueryQuery({
    skip: !query,
    variables: { filter: { q: query, per_page: 200 } },
  });
  const { data: selected, previousData: previousSelected } =
    GQL.useFindFoldersForQueryQuery({
      skip: ids.length === 0,
      variables: { ids, filter: { per_page: -1 } },
    });

  const options = useMemo(
    () =>
      (found?.findFolders.folders ?? []).map((f) => ({
        value: f.id,
        label: f.path,
      })),
    [found]
  );

  const values = useMemo(() => {
    const labels = new Map<string, string>();
    for (const f of [
      ...(previousSelected?.findFolders.folders ?? []),
      ...(selected?.findFolders.folders ?? []),
      ...(found?.findFolders.folders ?? []),
    ]) {
      labels.set(f.id, f.path);
    }
    return ids.map((id) => ({ value: id, label: labels.get(id) ?? id }));
  }, [ids, found, selected, previousSelected]);

  return (
    <Select
      inputId="library-limit-folders"
      className="react-select"
      classNamePrefix="react-select"
      isMulti
      options={options}
      value={values}
      onInputChange={setQuery}
      onChange={(v) => onChange(v.map((o) => o.value))}
      noOptionsMessage={() => null}
    />
  );
};

const LibraryLimitDialog: React.FC<{
  account: Account;
  onClose: (saved: boolean) => void;
}> = ({ account, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [setLimit, { loading }] = GQL.useUserSetLibraryLimitMutation();

  const existing = account.library_limit;
  const [limited, setLimited] = useState(!!existing);
  const [folderIDs, setFolderIDs] = useState(existing?.folder_ids ?? []);
  const [tagIDs, setTagIDs] = useState(existing?.tag_ids ?? []);
  const [studioIDs, setStudioIDs] = useState(existing?.studio_ids ?? []);

  async function save() {
    try {
      await setLimit({
        variables: {
          input: {
            name: account.name,
            library_limit: limited
              ? {
                  folder_ids: folderIDs,
                  tag_ids: tagIDs,
                  studio_ids: studioIDs,
                }
              : null,
          },
        },
      });
      onClose(true);
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
        text: intl.formatMessage({ id: "actions.save" }),
        onClick: save,
      }}
      cancel={{
        text: intl.formatMessage({ id: "actions.cancel" }),
        variant: "secondary",
        onClick: () => onClose(false),
      }}
    >
      <Form.Group>
        <Form.Check
          id="library-limit-enabled"
          type="switch"
          checked={limited}
          onChange={() => setLimited(!limited)}
          label={intl.formatMessage({
            id: "config.general.auth.library_limits.enabled",
          })}
        />
      </Form.Group>
      {limited && (
        <>
          <p>
            <FormattedMessage id="config.general.auth.library_limits.rule" />
          </p>
          <Form.Group>
            <Form.Label htmlFor="library-limit-folders">
              <FormattedMessage id="folders" />
            </Form.Label>
            <FolderIDSelect ids={folderIDs} onChange={setFolderIDs} />
          </Form.Group>
          <Form.Group>
            <Form.Label>
              <FormattedMessage id="tags" />
            </Form.Label>
            <TagIDSelect
              isMulti
              ids={tagIDs}
              onSelect={(items) => setTagIDs(items.map((o) => o.id))}
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>
              <FormattedMessage id="studios" />
            </Form.Label>
            <StudioIDSelect
              isMulti
              ids={studioIDs}
              onSelect={(items) => setStudioIDs(items.map((o) => o.id))}
            />
          </Form.Group>
        </>
      )}
    </ModalComponent>
  );
};

export const LibraryLimits: React.FC = () => {
  const intl = useIntl();
  const { data, loading, refetch } = GQL.useFindUsersForLibraryLimitsQuery();
  const [editing, setEditing] = useState<Account>();

  function canBeLimited(account: Account) {
    return (
      !account.roles?.includes(GQL.RoleEnum.Admin) &&
      !account.roles?.includes(GQL.RoleEnum.Modify)
    );
  }

  function describe(account: Account) {
    const limit = account.library_limit;
    if (limit) {
      return intl.formatMessage(
        { id: "config.general.auth.library_limits.limited_to" },
        {
          folders: limit.folder_ids.length,
          tags: limit.tag_ids.length,
          studios: limit.studio_ids.length,
        }
      );
    }

    return intl.formatMessage({
      id: canBeLimited(account)
        ? "config.general.auth.library_limits.whole_library"
        : "config.general.auth.library_limits.cannot_limit",
    });
  }

  return (
    <SettingSection
      id="library-limits"
      headingID="config.general.auth.library_limits.heading"
      subHeadingID="config.general.auth.library_limits.description"
    >
      {loading && <LoadingIndicator inline small message="" />}
      {data?.users.map((account) => (
        <div className="setting" key={account.name}>
          <div>
            <h3>{account.name}</h3>
            <div className="sub-heading">{describe(account)}</div>
          </div>
          <div>
            <Button
              disabled={!canBeLimited(account)}
              onClick={() => setEditing(account)}
            >
              <FormattedMessage id="actions.edit" />
            </Button>
          </div>
        </div>
      ))}
      {editing && (
        <LibraryLimitDialog
          account={editing}
          onClose={(saved) => {
            setEditing(undefined);
            if (saved) refetch();
          }}
        />
      )}
    </SettingSection>
  );
};
