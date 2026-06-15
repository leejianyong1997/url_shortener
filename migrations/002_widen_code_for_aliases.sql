-- Custom aliases can be longer than the original 7-char random codes.
-- The handler allows aliases up to 32 chars, so widen `code` to match and
-- prevent silent truncation. The UNIQUE index on `code` is preserved.

USE url_shortener;

ALTER TABLE links MODIFY code VARCHAR(32) NOT NULL;
