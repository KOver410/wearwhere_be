-- Soft-deletion anonymises email + phone (sets both NULL) for GDPR. The
-- original "email OR phone is required" check still applies to active rows
-- but must be skipped for already-deleted ones.
ALTER TABLE users DROP CONSTRAINT users_email_or_phone;
ALTER TABLE users ADD CONSTRAINT users_email_or_phone CHECK (
    deleted_at IS NOT NULL OR email IS NOT NULL OR phone IS NOT NULL
);
