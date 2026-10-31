import React, { useState } from "react";
import { useApolloClient } from "@apollo/client";
import { Button, Form, Modal } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { useToast } from "src/hooks/Toast";

type CorrectionField =
  | "horizontal_offset"
  | "vertical_offset"
  | "brightness"
  | "contrast"
  | "saturation";
type ChromaKeyField = "hue" | "saturation" | "value" | "threshold" | "opacity";
type NumericFields<T extends string> = Record<T, string>;

const correctionFields: CorrectionField[] = [
  "horizontal_offset",
  "vertical_offset",
  "brightness",
  "contrast",
  "saturation",
];
const chromaKeyFields: ChromaKeyField[] = [
  "hue",
  "saturation",
  "value",
  "threshold",
  "opacity",
];

function emptyFields<T extends string>(fields: T[]): NumericFields<T> {
  return Object.fromEntries(
    fields.map((field) => [field, ""])
  ) as NumericFields<T>;
}

function numericFields<T extends string>(
  fields: T[],
  values?: Partial<Record<T, number | null>> | null
): NumericFields<T> {
  return Object.fromEntries(
    fields.map((field) => [field, values?.[field] ?? ""])
  ) as NumericFields<T>;
}

function numericValues<T extends string>(fields: NumericFields<T>) {
  const values: Partial<Record<T, number>> = {};
  for (const [key, value] of Object.entries(fields) as [T, string][]) {
    if (value.trim() === "") continue;
    const number = Number(value);
    if (Number.isFinite(number)) values[key] = number;
  }
  return values;
}

function labelFor(value: string) {
  return value
    .split("_")
    .map((part) => part.charAt(0) + part.slice(1).toLowerCase())
    .join(" ");
}

interface IProps {
  file: GQL.VideoFileDataFragment;
}

export const VRMetadataEditor: React.FC<IProps> = ({ file }) => {
  const client = useApolloClient();
  const Toast = useToast();
  const [saveMetadata, { loading }] = GQL.useFileSetVrMetadataMutation();
  const [show, setShow] = useState(false);
  const [projection, setProjection] = useState<GQL.VideoProjection | "">("");
  const [stereoMode, setStereoMode] = useState<GQL.StereoMode | "">("");
  const [corrections, setCorrections] = useState<
    NumericFields<CorrectionField>
  >(emptyFields(correctionFields));
  const [chromaKeys, setChromaKeys] = useState<NumericFields<ChromaKeyField>[]>(
    [emptyFields(chromaKeyFields)]
  );
  const [alphaPacked, setAlphaPacked] = useState<"" | "true" | "false">("");

  function onOpen() {
    setProjection(file.projection ?? "");
    setStereoMode(file.stereo_mode ?? "");
    setCorrections(numericFields(correctionFields, file.vr_corrections));
    const existingKeys = (file.vr_passthrough?.chroma_keys ?? []).map((key) =>
      numericFields(chromaKeyFields, key)
    );
    setChromaKeys(
      existingKeys?.length ? existingKeys : [emptyFields(chromaKeyFields)]
    );
    setAlphaPacked(
      file.vr_passthrough?.alpha_packed == null
        ? ""
        : file.vr_passthrough.alpha_packed
          ? "true"
          : "false"
    );
    setShow(true);
  }

  function setCorrection(field: CorrectionField, value: string) {
    setCorrections((current) => ({ ...current, [field]: value }));
  }

  function setChromaKey(index: number, field: ChromaKeyField, value: string) {
    setChromaKeys((current) =>
      current.map((key, keyIndex) =>
        keyIndex === index ? { ...key, [field]: value } : key
      )
    );
  }

  async function onSave() {
    const correctionValues = numericValues(corrections);
    const chromaKeyValues = chromaKeys
      .map(numericValues)
      .filter((key) => Object.keys(key).length > 0);
    const passthrough = {
      alpha_packed: alphaPacked === "" ? null : alphaPacked === "true",
      chroma_keys: chromaKeyValues,
    };
    const input: GQL.FileSetVrMetadataInput = {
      id: file.id,
      projection: projection || null,
      stereo_mode: stereoMode || null,
      vr_corrections:
        Object.keys(correctionValues).length > 0 ? correctionValues : null,
      vr_passthrough:
        alphaPacked !== "" || chromaKeyValues.length > 0 ? passthrough : null,
    };

    try {
      await saveMetadata({ variables: { input } });
      const cacheID = client.cache.identify({
        __typename: "VideoFile",
        id: file.id,
      });
      if (cacheID) {
        client.cache.modify({
          id: cacheID,
          fields: {
            projection: () => projection || null,
            stereo_mode: () => stereoMode || null,
            vr_corrections: () =>
              Object.keys(correctionValues).length > 0
                ? { __typename: "VRCorrections", ...correctionValues }
                : null,
            vr_passthrough: () =>
              input.vr_passthrough
                ? {
                    __typename: "VRPassthrough",
                    alpha_packed: passthrough.alpha_packed,
                    chroma_keys: chromaKeyValues.map((key) => ({
                      __typename: "VRChromaKey",
                      ...key,
                    })),
                  }
                : null,
          },
        });
      }
      Toast.success("VR metadata saved");
      setShow(false);
    } catch (error) {
      Toast.error(error);
    }
  }

  function renderNumberField(
    label: string,
    value: string,
    onChange: (value: string) => void
  ) {
    return (
      <Form.Group key={label} className="mb-2">
        <Form.Label>{labelFor(label)}</Form.Label>
        <Form.Control
          type="number"
          step="any"
          value={value}
          onChange={(event) => onChange(event.currentTarget.value)}
        />
      </Form.Group>
    );
  }

  const correctionSummary = file.vr_corrections
    ? correctionFields
        .filter((field) => file.vr_corrections?.[field] != null)
        .map((field) => `${labelFor(field)}: ${file.vr_corrections?.[field]}`)
        .join(", ")
    : "";
  const passthroughSummary = file.vr_passthrough
    ? `${file.vr_passthrough.chroma_keys?.length ?? 0} chroma key(s), alpha packed: ${
        file.vr_passthrough.alpha_packed == null
          ? "unspecified"
          : file.vr_passthrough.alpha_packed
            ? "yes"
            : "no"
      }`
    : "";

  return (
    <>
      <dt>VR metadata</dt>
      <dd>
        {correctionSummary && <div>{correctionSummary}</div>}
        {passthroughSummary && <div>{passthroughSummary}</div>}
        <Button size="sm" variant="secondary" onClick={onOpen}>
          <FormattedMessage id="media_info.edit_vr_metadata" />
        </Button>
      </dd>

      <Modal show={show} onHide={() => setShow(false)} size="lg">
        <Modal.Header closeButton>
          <Modal.Title>
            <FormattedMessage id="media_info.edit_vr_metadata" />
          </Modal.Title>
        </Modal.Header>
        <Modal.Body>
          <Form.Group>
            <Form.Label>
              <FormattedMessage id="media_info.projection" />
            </Form.Label>
            <Form.Control
              as="select"
              value={projection}
              onChange={(event) =>
                setProjection(
                  event.currentTarget.value as GQL.VideoProjection | ""
                )
              }
            >
              <option value="">Unspecified</option>
              {Object.values(GQL.VideoProjection).map((value) => (
                <option key={value} value={value}>
                  {labelFor(value)}
                </option>
              ))}
            </Form.Control>
          </Form.Group>
          <Form.Group>
            <Form.Label>
              <FormattedMessage id="media_info.stereo_mode" />
            </Form.Label>
            <Form.Control
              as="select"
              value={stereoMode}
              onChange={(event) =>
                setStereoMode(event.currentTarget.value as GQL.StereoMode | "")
              }
            >
              <option value="">Unspecified</option>
              {Object.values(GQL.StereoMode).map((value) => (
                <option key={value} value={value}>
                  {labelFor(value)}
                </option>
              ))}
            </Form.Control>
          </Form.Group>
          <h5>
            <FormattedMessage id="media_info.vr_corrections" />
          </h5>
          <div className="row">
            {correctionFields.map((field) => (
              <div className="col-md-6" key={field}>
                {renderNumberField(field, corrections[field], (value) =>
                  setCorrection(field, value)
                )}
              </div>
            ))}
          </div>
          <h5>
            <FormattedMessage id="media_info.passthrough" />
          </h5>
          <Form.Group>
            <Form.Label>
              <FormattedMessage id="media_info.alpha_packed" />
            </Form.Label>
            <Form.Control
              as="select"
              value={alphaPacked}
              onChange={(event) =>
                setAlphaPacked(
                  event.currentTarget.value as "" | "true" | "false"
                )
              }
            >
              <option value="">Unspecified</option>
              <option value="true">Yes</option>
              <option value="false">No</option>
            </Form.Control>
          </Form.Group>
          {chromaKeys.map((key, index) => (
            <div className="border rounded p-2 mb-2" key={index}>
              <div className="d-flex justify-content-between align-items-center">
                <strong>
                  <FormattedMessage
                    id="media_info.chroma_key_number"
                    values={{ number: index + 1 }}
                  />
                </strong>
                <Button
                  size="sm"
                  variant="link"
                  onClick={() =>
                    setChromaKeys((current) =>
                      current.filter((_, keyIndex) => keyIndex !== index)
                    )
                  }
                >
                  <FormattedMessage id="actions.remove" />
                </Button>
              </div>
              <div className="row">
                {chromaKeyFields.map((field) => (
                  <div className="col-md-4" key={field}>
                    {renderNumberField(field, key[field], (value) =>
                      setChromaKey(index, field, value)
                    )}
                  </div>
                ))}
              </div>
            </div>
          ))}
          <Button
            size="sm"
            variant="secondary"
            disabled={chromaKeys.length >= 4}
            onClick={() =>
              setChromaKeys((current) => [
                ...current,
                emptyFields(chromaKeyFields),
              ])
            }
          >
            <FormattedMessage id="media_info.add_chroma_key" />
          </Button>
        </Modal.Body>
        <Modal.Footer>
          <Button variant="secondary" onClick={() => setShow(false)}>
            <FormattedMessage id="actions.cancel" />
          </Button>
          <Button variant="primary" disabled={loading} onClick={onSave}>
            <FormattedMessage id="actions.save" />
          </Button>
        </Modal.Footer>
      </Modal>
    </>
  );
};
