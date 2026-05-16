ALTER TABLE users DROP CONSTRAINT users_email_or_phone;
ALTER TABLE users ADD CONSTRAINT users_email_or_phone CHECK (email IS NOT NULL OR phone IS NOT NULL);
