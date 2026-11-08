ALTER TABLE session.authorization_codes
    ADD COLUMN nonce TEXT NOT NULL DEFAULT '';
