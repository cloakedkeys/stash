import React from "react";
import { Link } from "react-router-dom";
import { useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import { objectTitle } from "src/core/files";
import { RatingSystem } from "../Shared/Rating/RatingSystem";
import { useStoryUserDataUpdate } from "src/core/StashService";
import { IColumn, ListTable } from "../List/ListTable";
import { useTableColumns } from "src/hooks/useTableColumns";
import { FileSize } from "../Shared/FileSize";

interface IStoryListTableProps {
  stories: GQL.SlimStoryDataFragment[];
  selectedIds: Set<string>;
  onSelectChange: (id: string, selected: boolean, shiftKey: boolean) => void;
}

const TABLE_NAME = "stories";

export const StoryListTable: React.FC<IStoryListTableProps> = (props) => {
  const intl = useIntl();
  const [setStoryUserData] = useStoryUserDataUpdate();

  function setRating(v: number | null, storyId: string) {
    setStoryUserData({
      variables: { input: { id: storyId, rating100: v } },
    });
  }

  interface IColumnSpec {
    value: string;
    label: string;
    defaultShow?: boolean;
    mandatory?: boolean;
    render: (story: GQL.SlimStoryDataFragment) => React.ReactNode;
  }

  const allColumns: IColumnSpec[] = [
    {
      value: "title",
      label: intl.formatMessage({ id: "title" }),
      defaultShow: true,
      mandatory: true,
      render: (s) => {
        const title = objectTitle(s);
        return (
          <Link to={`/stories/${s.id}`} title={title}>
            <span className="ellips-data">{title}</span>
          </Link>
        );
      },
    },
    {
      value: "author",
      label: intl.formatMessage({ id: "author" }),
      defaultShow: true,
      render: (s) => <span className="ellips-data">{s.author}</span>,
    },
    {
      value: "date",
      label: intl.formatMessage({ id: "date" }),
      defaultShow: true,
      render: (s) => <>{s.date}</>,
    },
    {
      value: "rating",
      label: intl.formatMessage({ id: "rating" }),
      defaultShow: true,
      render: (s) => (
        <RatingSystem
          value={s.rating100}
          onSetRating={(value) => setRating(value, s.id)}
          clickToRate
        />
      ),
    },
    {
      value: "tags",
      label: intl.formatMessage({ id: "tags" }),
      defaultShow: true,
      render: (s) => (
        <ul className="comma-list overflowable">
          {s.tags.map((tag) => (
            <li key={tag.id}>
              <Link to={`/tags/${tag.id}`}>
                <span>{tag.name}</span>
              </Link>
            </li>
          ))}
        </ul>
      ),
    },
    {
      value: "path",
      label: intl.formatMessage({ id: "path" }),
      render: (s) => (
        <ul className="newline-list overflowable TruncatedText">
          {s.files.map((file) => (
            <li key={file.id}>
              <span>{file.path}</span>
            </li>
          ))}
        </ul>
      ),
    },
    {
      value: "filesize",
      label: intl.formatMessage({ id: "filesize" }),
      render: (s) => (
        <ul className="comma-list">
          {s.files.map((file) => (
            <li key={file.id}>
              <FileSize size={file.size} />
            </li>
          ))}
        </ul>
      ),
    },
  ];

  const defaultColumns = allColumns
    .filter((col) => col.defaultShow)
    .map((col) => col.value);

  const { selectedColumns, saveColumns } = useTableColumns(
    TABLE_NAME,
    defaultColumns
  );

  function renderCell(column: IColumn, story: GQL.SlimStoryDataFragment) {
    return allColumns.find((c) => c.value === column.value)?.render(story);
  }

  return (
    <ListTable
      className="story-table"
      items={props.stories}
      allColumns={allColumns}
      columns={selectedColumns}
      setColumns={(c) => saveColumns(c)}
      selectedIds={props.selectedIds}
      onSelectChange={props.onSelectChange}
      renderCell={renderCell}
    />
  );
};
