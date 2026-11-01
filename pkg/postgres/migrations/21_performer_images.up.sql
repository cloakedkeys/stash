CREATE TABLE performer_images (
  performer_id INTEGER NOT NULL REFERENCES performers(id) ON DELETE CASCADE,
  image_blob VARCHAR(255) NOT NULL REFERENCES blobs(checksum),
  position INTEGER NOT NULL,
  PRIMARY KEY (performer_id, image_blob)
);

CREATE INDEX performer_images_image_blob_idx ON performer_images(image_blob);
