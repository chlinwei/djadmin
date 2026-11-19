DO $$
BEGIN
  RAISE EXCEPTION '000067 merges issue identities irreversibly; restore a pre-migration backup to downgrade';
END $$;
