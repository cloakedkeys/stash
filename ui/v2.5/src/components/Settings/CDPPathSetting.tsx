import React from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { ModalSetting } from "./Inputs";

const CDPPathField: React.FC<{
  value: string | undefined;
  setValue: (v?: string) => void;
}> = ({ value, setValue }) => {
  const [validate, { data, loading }] = GQL.useValidateCdpLazyQuery({
    fetchPolicy: "network-only",
  });

  const result = data?.validateCDPPath;

  return (
    <>
      <Form.Control
        className="text-input"
        value={value ?? ""}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          setValue(e.currentTarget.value)
        }
      />
      <Form.Group className="mt-2 mb-0">
        <Button
          variant="secondary"
          disabled={loading}
          onClick={() => validate({ variables: { path: value ?? "" } })}
          className="mr-3"
        >
          <FormattedMessage id="config.general.chrome_cdp_path_test" />
        </Button>
        {result && !loading && (
          <b className={result.valid ? "text-success" : "text-danger"}>
            {result.status}
          </b>
        )}
      </Form.Group>
    </>
  );
};

interface ICDPPathSetting {
  value: string | undefined;
  onChange: (v: string) => void;
}

// Chrome CDP path, with a button in the edit dialog to test the path
// before saving it.
export const CDPPathSetting: React.FC<ICDPPathSetting> = ({
  value,
  onChange,
}) => {
  return (
    <ModalSetting<string>
      id="scraperCDPPath"
      headingID="config.general.chrome_cdp_path"
      subHeadingID="config.general.chrome_cdp_path_desc"
      value={value}
      onChange={onChange}
      renderField={(v, setValue) => (
        <CDPPathField value={v} setValue={setValue} />
      )}
      renderValue={(v) => <span>{v}</span>}
    />
  );
};
