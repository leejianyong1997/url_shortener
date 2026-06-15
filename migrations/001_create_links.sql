-- Run this once to set up the database and table.
-- In XAMPP: paste into phpMyAdmin's SQL tab, or pipe via the mysql CLI:
--   mysql -u root < migrations/001_create_links.sql

CREATE DATABASE IF NOT EXISTS url_shortener
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

USE url_shortener;

CREATE TABLE IF NOT EXISTS links (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(16) NOT NULL UNIQUE,        -- random base62 short code
  long_url    TEXT        NOT NULL,               -- the original URL
  clicks      BIGINT UNSIGNED NOT NULL DEFAULT 0, -- visit counter
  created_at  TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
);
