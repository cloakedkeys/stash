import React, { useState } from "react";
import { useIntl } from "react-intl";
import uniq from "lodash-es/uniq";
import * as GQL from "src/core/generated-graphql";
import {
  ScrapedInputGroupRow,
  ScrapedStringListRow,
  ScrapedTextAreaRow,
} from "src/components/Shared/ScrapeDialog/ScrapeDialogRow";
import { ScrapeDialog } from "src/components/Shared/ScrapeDialog/ScrapeDialog";
import { ScrapeResult } from "src/components/Shared/ScrapeDialog/scrapeResult";
import { useScrapedTags } from "src/components/Shared/ScrapeDialog/scrapedTags";
import { Tag } from "src/components/Tags/TagSelect";

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
  scraped: GQL.ScrapedStoryDataFragment;
  onClose: (scraped?: GQL.ScrapedStoryDataFragment) => void;
}

// StoryScrapeDialog shows what a scraper found beside the story's current
// details, so each field can be kept or replaced.
export const StoryScrapeDialog: React.FC<IStoryScrapeDialogProps> = ({
  story,
  storyTags,
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
      tags: tags.getNewValue(),
    };
  }

  if (linkDialog) {
    return linkDialog;
  }

  return (
    <ScrapeDialog
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
    </ScrapeDialog>
  );
};
