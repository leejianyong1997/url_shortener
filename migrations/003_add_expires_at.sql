-- Optional link expiration. NULL means the link never expires.

USE url_shortener;

ALTER TABLE links
  ADD COLUMN expires_at TIMESTAMP NULL DEFAULT NULL AFTER created_at;
