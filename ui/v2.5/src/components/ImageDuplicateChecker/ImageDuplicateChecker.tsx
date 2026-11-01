import React, { useState } from "react";
import { Button, Card, Form, Table } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { Link } from "react-router-dom";
import * as GQL from "src/core/generated-graphql";
import { DeleteImagesDialog } from "../Images/DeleteImagesDialog";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { ErrorMessage } from "../Shared/ErrorMessage";
import { Pagination } from "../List/Pagination";
import { objectTitle } from "src/core/files";

const pageSize = 20;

const ImageDuplicateChecker: React.FC = () => {
  const intl = useIntl();
  const [distance, setDistance] = useState(0);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<GQL.SlimImageDataFragment[]>();
  const { data, loading, error, refetch } = GQL.useFindDuplicateImagesQuery({
    fetchPolicy: "no-cache",
    variables: { distance },
  });
  if (loading) return <LoadingIndicator />;
  if (error) return <ErrorMessage error={error.message} />;
  const groups = data?.findDuplicateImages ?? [];
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(groups.length / pageSize))
  );

  return (
    <Card className="col col-xl-12 mx-auto p-3">
      <h4>
        <FormattedMessage id="config.tools.image_duplicate_checker" />
      </h4>
      <p>
        <FormattedMessage id="image_dupe_check.description" />
      </p>
      <Form.Group controlId="image-duplicate-distance">
        <Form.Label>
          <FormattedMessage id="dupe_check.search_accuracy_label" />
        </Form.Label>
        <Form.Control
          as="select"
          value={distance}
          onChange={(event) => {
            setDistance(Number(event.currentTarget.value));
            setPage(1);
          }}
        >
          {[
            ["exact", 0],
            ["high", 4],
            ["medium", 8],
            ["low", 10],
          ].map(([label, value]) => (
            <option key={value} value={value}>
              {intl.formatMessage({ id: "dupe_check.options." + label })}
            </option>
          ))}
        </Form.Control>
      </Form.Group>
      <p>
        <FormattedMessage
          id="dupe_check.found_sets"
          values={{ setCount: groups.length }}
        />
      </p>
      {selected && (
        <DeleteImagesDialog
          selected={selected}
          onClose={(deleted) => {
            setSelected(undefined);
            if (deleted) void refetch();
          }}
        />
      )}
      {groups
        .slice((currentPage - 1) * pageSize, currentPage * pageSize)
        .map((group) => (
          <Table
            responsive
            striped
            key={group.map((image) => image.id).join("-")}
          >
            <thead>
              <tr>
                <th>{intl.formatMessage({ id: "image" })}</th>
                <th>{intl.formatMessage({ id: "files" })}</th>
                <th>{intl.formatMessage({ id: "actions.delete" })}</th>
              </tr>
            </thead>
            <tbody>
              {group.map((image) => (
                <tr key={image.id}>
                  <td>
                    <Link to={"/images/" + image.id}>
                      <img
                        src={image.paths.thumbnail ?? ""}
                        alt=""
                        width={100}
                      />
                      <span className="ml-2">
                        {objectTitle({
                          title: image.title,
                          files: image.visual_files,
                        })}
                      </span>
                    </Link>
                  </td>
                  <td className="text-break">
                    {image.visual_files.map((file) => (
                      <div key={file.id}>{file.path}</div>
                    ))}
                  </td>
                  <td>
                    <Button
                      variant="danger"
                      onClick={() => setSelected([image])}
                    >
                      <FormattedMessage id="actions.delete" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        ))}
      <Pagination
        itemsPerPage={pageSize}
        currentPage={currentPage}
        totalItems={groups.length}
        metadataByline={[]}
        onChangePage={setPage}
      />
    </Card>
  );
};

export default ImageDuplicateChecker;
