-- The text of a story that has no text file, such as one scraped from a
-- website. Kept apart from the stories table so that lists of stories don't
-- load it.
CREATE TABLE stories_text (
    story_id integer NOT NULL PRIMARY KEY REFERENCES stories(id) ON DELETE CASCADE,
    text text NOT NULL
);
