-- Stash's filters translate patterns and use ~ directly. This function stays for plugins that
-- call it through raw SQL, whose patterns reach PostgreSQL without being translated.
CREATE OR REPLACE FUNCTION regex_match(text, text)
RETURNS boolean AS $$
BEGIN
    RETURN COALESCE($1 ~ $2, false);
EXCEPTION WHEN invalid_regular_expression THEN
    RAISE EXCEPTION 'regex_match: "%" is not valid in PostgreSQL''s regular expression syntax, which is what raw SQL uses on PostgreSQL. Stash''s regex filters accept Go''s syntax on every database. (%)', $2, SQLERRM
        USING ERRCODE = 'invalid_regular_expression';
END
$$ LANGUAGE plpgsql IMMUTABLE;

-- Nothing here needs plperl any more. Leave it if another account installed it, or if something
-- else in the database uses it.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'plperl' AND extowner = current_user::regrole) THEN
        DROP EXTENSION plperl;
    END IF;
EXCEPTION WHEN dependent_objects_still_exist OR insufficient_privilege THEN
    RAISE NOTICE 'keeping plperl: %', SQLERRM;
END
$$;
