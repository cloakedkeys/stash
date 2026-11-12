import React, { useState } from "react";
import { useIntl } from "react-intl";
import uniq from "lodash-es/uniq";
import { FormControl } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import {
  ScrapeDialogRow,
  ScrapedInputGroupRow,
  ScrapedStringListRow,
  ScrapedTextAreaRow,
} from "src/components/Shared/ScrapeDialog/ScrapeDialogRow";
import { ScrapeDialog } from "src/components/Shared/ScrapeDialog/ScrapeDialog";
import { ScrapeResult } from "src/components/Shared/ScrapeDialog/scrapeResult";
import { useScrapedTags } from "src/components/Shared/ScrapeDialog/scrapedTags";
import { Tag } from "src/components/Tags/TagSelect";
import { storyTextLength } from "./storyScrape";

export interface IStoryFields {
  title?: string;
  author?: string;
  source_website?: string;
  urls?: string[];
  date?: string;
  language?: string;
  tagline?: string;
  details?: string;
}

interface IStoryScrapeDialogProps {
  story: IStoryFields;
  storyTags: Tag[];
  // a story with a text file always shows the file's text, so it isn't offered
  // a scraped text
  hasFile: boolean;
  // whether the story already has text stored with it
  hasText: boolean;
  scraped: GQL.ScrapedStoryDataFragment;
  onClose: (scraped?: GQL.ScrapedStoryDataFragment) => void;
}

// The story's text is too long to show here, so its row shows how long it is.
const ScrapedTextLengthRow: React.FC<{
  hasText: boolean;
  result: ScrapeResult<string>;
  onChange: (value: ScrapeResult<string>) => void;
}> = ({ hasText, result, onChange }) => {
  const intl = useIntl();

  function describe(text?: string) {
    if (!text) return intl.formatMessage({ id: "story_text.none" });
    const { words, characters } = storyTextLength(text);
    return intl.formatMessage(
      { id: "story_text.length" },
      { words, characters }
    );
  }

  const field = (value: string) => (
    <FormControl
      value={value}
      readOnly
      className="bg-secondary text-white border-secondary"
    />
  );

  return (
    <ScrapeDialogRow
      field="text"
      title={intl.formatMessage({ id: "story_text.title" })}
      result={result}
      originalField={field(
        hasText
          ? intl.formatMessage({ id: "story_text.stored" })
          : describe(undefined)
      )}
      newField={field(describe(result.newValue))}
      onChange={onChange}
    />
  );
};

// StoryScrapeDialog shows what a scraper found beside the story's current
// details, so each field can be kept or replaced.
export const StoryScrapeDialog: React.FC<IStoryScrapeDialogProps> = ({
  story,
  storyTags,
  hasFile,
  hasText,
  scraped,
  onClose,
}) => {
  const intl = useIntl();

  const useField = (current?: string, found?: string | null) =>
    useState(new ScrapeResult<string>(current, found));

  const [title, setTitle] = useField(story.title, scraped.title);
  const [author, setAuthor] = useField(story.author, scraped.author);
  const [sourceWebsite, setSourceWebsite] = useField(
    story.source_website,
    scraped.source_website
  );
  const [date, setDate] = useField(story.date, scraped.date);
  const [language, setLanguage] = useField(story.language, scraped.language);
  const [tagline, setTagline] = useField(story.tagline, scraped.tagline);
  const [details, setDetails] = useField(story.details, scraped.details);
  // the stored text isn't loaded, so the scraped text is always new to the dialog
  const [text, setText] = useField(
    undefined,
    hasFile ? undefined : scraped.text
  );
  const [urls, setURLs] = useState(
    new ScrapeResult<string[]>(
      story.urls,
      scraped.urls
        ? uniq((story.urls ?? []).concat(scraped.urls ?? []))
        : undefined
    )
  );

  const { tags, newTags, scrapedTagsRow, linkDialog } = useScrapedTags(
    storyTags,
    scraped.tags
  );

  // nothing found, so there's nothing to choose from
  if (
    [
      title,
      author,
      sourceWebsite,
      urls,
      date,
      language,
      tagline,
      details,
      text,
      tags,
    ].every((r) => !r.scraped) &&
    newTags.length === 0
  ) {
    onClose();
    return null;
  }

  function makeNewScrapedItem(): GQL.ScrapedStoryDataFragment {
    return {
      title: title.getNewValue(),
      author: author.getNewValue(),
      source_website: sourceWebsite.getNewValue(),
      urls: urls.getNewValue(),
      date: date.getNewValue(),
      language: language.getNewValue(),
      tagline: tagline.getNewValue(),
      details: details.getNewValue(),
      text: text.getNewValue(),
      tags: tags.getNewValue(),
    };
  }

  if (linkDialog) {
    return linkDialog;
  }

  return (
    <ScrapeDialog
      scrapeType="story"
      title={intl.formatMessage(
        { id: "dialogs.scrape_entity_title" },
        { entity_type: intl.formatMessage({ id: "story" }) }
      )}
      onClose={(apply) => onClose(apply ? makeNewScrapedItem() : undefined)}
    >
      <ScrapedInputGroupRow
        field="title"
        title={intl.formatMessage({ id: "title" })}
        result={title}
        onChange={setTitle}
      />
      <ScrapedInputGroupRow
        field="author"
        title={intl.formatMessage({ id: "author" })}
        result={author}
        onChange={setAuthor}
      />
      <ScrapedInputGroupRow
        field="source_website"
        title={intl.formatMessage({ id: "source_website" })}
        result={sourceWebsite}
        onChange={setSourceWebsite}
      />
      <ScrapedStringListRow
        field="urls"
        title={intl.formatMessage({ id: "urls" })}
        result={urls}
        onChange={setURLs}
      />
      <ScrapedInputGroupRow
        field="date"
        title={intl.formatMessage({ id: "date" })}
        placeholder="YYYY-MM-DD"
        result={date}
        onChange={setDate}
      />
      <ScrapedInputGroupRow
        field="language"
        title={intl.formatMessage({ id: "language" })}
        result={language}
        onChange={setLanguage}
      />
      {scrapedTagsRow}
      <ScrapedInputGroupRow
        field="tagline"
        title={intl.formatMessage({ id: "tagline" })}
        result={tagline}
        onChange={setTagline}
      />
      <ScrapedTextAreaRow
        field="details"
        title={intl.formatMessage({ id: "details" })}
        result={details}
        onChange={setDetails}
      />
      <ScrapedTextLengthRow
        hasText={hasText}
        result={text}
        onChange={setText}
      />
    </ScrapeDialog>
  );
};
