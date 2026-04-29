-- Users
DEFINE TABLE IF NOT EXISTS users SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS email ON users TYPE string ASSERT string::is_email($value);
DEFINE FIELD IF NOT EXISTS password_hash ON users TYPE string;
DEFINE FIELD IF NOT EXISTS name ON users TYPE string;
DEFINE FIELD IF NOT EXISTS skills ON users TYPE array<string> DEFAULT [];
DEFINE FIELD IF NOT EXISTS bio ON users TYPE string DEFAULT '';
DEFINE FIELD IF NOT EXISTS categories ON users TYPE array<string> DEFAULT [];
DEFINE FIELD IF NOT EXISTS intensity ON users TYPE string DEFAULT 'low' ASSERT $value IN ['low', 'medium', 'high'];
DEFINE FIELD IF NOT EXISTS phone ON users TYPE option<string>;
DEFINE FIELD IF NOT EXISTS xp ON users TYPE int DEFAULT 0;
DEFINE FIELD IF NOT EXISTS instagram ON users TYPE option<string>;
DEFINE FIELD IF NOT EXISTS linkedin ON users TYPE option<string>;
DEFINE FIELD IF NOT EXISTS s3_pfp ON users TYPE string DEFAULT '';
DEFINE FIELD IF NOT EXISTS s3_pdf ON users TYPE string DEFAULT '';
DEFINE FIELD IF NOT EXISTS badges ON users TYPE array<string> DEFAULT [];
DEFINE FIELD IF NOT EXISTS notification_settings ON users TYPE object DEFAULT { application_accepted: true, opportunity_reminder: true, application_declined: true, opportunity_canceled: true };
DEFINE FIELD IF NOT EXISTS notification_settings.application_accepted ON users TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS notification_settings.opportunity_reminder ON users TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS notification_settings.application_declined ON users TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS notification_settings.opportunity_canceled ON users TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS created_at ON users TYPE datetime;
DEFINE FIELD IF NOT EXISTS updated_at ON users TYPE datetime;
DEFINE INDEX IF NOT EXISTS users_email ON users FIELDS email UNIQUE;

-- Orgs
DEFINE TABLE IF NOT EXISTS orgs SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS name ON orgs TYPE string;
DEFINE FIELD IF NOT EXISTS password_hash ON orgs TYPE string;
DEFINE FIELD IF NOT EXISTS categories ON orgs TYPE array<string> DEFAULT [];
DEFINE FIELD IF NOT EXISTS description ON orgs TYPE string;
DEFINE FIELD IF NOT EXISTS website ON orgs TYPE option<string>;
DEFINE FIELD IF NOT EXISTS email ON orgs TYPE string ASSERT string::is_email($value);
DEFINE FIELD IF NOT EXISTS phone ON orgs TYPE option<string>;
DEFINE FIELD IF NOT EXISTS address ON orgs TYPE option<string>;
DEFINE FIELD IF NOT EXISTS location ON orgs TYPE string;
DEFINE FIELD IF NOT EXISTS instagram ON orgs TYPE option<string>;
DEFINE FIELD IF NOT EXISTS linkedin ON orgs TYPE option<string>;
DEFINE FIELD IF NOT EXISTS s3_pfp ON orgs TYPE string DEFAULT '';
DEFINE FIELD IF NOT EXISTS verified ON orgs TYPE bool DEFAULT false;
DEFINE FIELD IF NOT EXISTS notification_settings ON orgs TYPE object DEFAULT { applicant_digest: true, applicant_digest_frequency: 'daily', accepted_withdrawal: true };
DEFINE FIELD IF NOT EXISTS notification_settings.applicant_digest ON orgs TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS notification_settings.applicant_digest_frequency ON orgs TYPE string DEFAULT 'daily' ASSERT $value IN ['daily', 'weekly'];
DEFINE FIELD IF NOT EXISTS notification_settings.accepted_withdrawal ON orgs TYPE bool DEFAULT true;
DEFINE FIELD IF NOT EXISTS applicant_digest_sent_at ON orgs TYPE option<datetime>;
DEFINE FIELD IF NOT EXISTS created_at ON orgs TYPE datetime;
DEFINE FIELD IF NOT EXISTS updated_at ON orgs TYPE datetime;
DEFINE INDEX IF NOT EXISTS orgs_email ON orgs FIELDS email UNIQUE;

-- Admins
DEFINE TABLE IF NOT EXISTS admins SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS email ON admins TYPE string ASSERT string::is_email($value);
DEFINE FIELD IF NOT EXISTS password_hash ON admins TYPE string;
DEFINE FIELD IF NOT EXISTS name ON admins TYPE string;
DEFINE FIELD IF NOT EXISTS created_at ON admins TYPE datetime;
DEFINE FIELD IF NOT EXISTS updated_at ON admins TYPE datetime;
DEFINE INDEX IF NOT EXISTS admins_email ON admins FIELDS email UNIQUE;

-- Opportunities
DEFINE TABLE IF NOT EXISTS opportunities SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS org_id ON opportunities TYPE record<orgs>;
DEFINE FIELD IF NOT EXISTS title ON opportunities TYPE string;
DEFINE FIELD IF NOT EXISTS category ON opportunities TYPE string;
DEFINE FIELD IF NOT EXISTS difficulty ON opportunities TYPE int DEFAULT 0 ASSERT $value IN [0, 1, 2];
DEFINE FIELD IF NOT EXISTS description ON opportunities TYPE string;
DEFINE FIELD IF NOT EXISTS date ON opportunities TYPE datetime;
DEFINE FIELD OVERWRITE duration ON opportunities TYPE number ASSERT $value > 0;
DEFINE FIELD IF NOT EXISTS location ON opportunities TYPE string;
DEFINE FIELD IF NOT EXISTS max_spots ON opportunities TYPE int ASSERT $value > 0;
DEFINE FIELD OVERWRITE spots_left ON opportunities TYPE int ASSERT $value >= 0;
DEFINE FIELD IF NOT EXISTS recurring ON opportunities TYPE option<string>;
DEFINE FIELD IF NOT EXISTS drop_in ON opportunities TYPE bool DEFAULT false;
DEFINE FIELD IF NOT EXISTS event_link ON opportunities TYPE option<string>;
DEFINE FIELD IF NOT EXISTS resources_link ON opportunities TYPE option<string>;
DEFINE FIELD IF NOT EXISTS tags ON opportunities TYPE array<string> DEFAULT [];
DEFINE FIELD IF NOT EXISTS created_at ON opportunities TYPE datetime;
DEFINE FIELD IF NOT EXISTS updated_at ON opportunities TYPE datetime;

-- Applications
DEFINE TABLE IF NOT EXISTS applications SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS user_id ON applications TYPE record<users>;
DEFINE FIELD IF NOT EXISTS opportunity_id ON applications TYPE record<opportunities>;
DEFINE FIELD IF NOT EXISTS status ON applications TYPE string DEFAULT 'pending' ASSERT $value IN ['pending', 'accepted', 'rejected'];
DEFINE FIELD OVERWRITE xp_awarded ON applications TYPE bool DEFAULT false;
DEFINE FIELD IF NOT EXISTS reminder_sent_at ON applications TYPE option<datetime>;
DEFINE FIELD IF NOT EXISTS created_at ON applications TYPE datetime;
DEFINE FIELD IF NOT EXISTS updated_at ON applications TYPE datetime;
DEFINE INDEX IF NOT EXISTS applications_unique ON applications FIELDS user_id, opportunity_id UNIQUE;

-- Refresh Tokens
DEFINE TABLE IF NOT EXISTS refresh_tokens SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS token_hash ON refresh_tokens TYPE string;
DEFINE FIELD IF NOT EXISTS user_id ON refresh_tokens TYPE string;
DEFINE FIELD IF NOT EXISTS expires_at ON refresh_tokens TYPE datetime;
DEFINE FIELD IF NOT EXISTS created_at ON refresh_tokens TYPE datetime;
DEFINE FIELD IF NOT EXISTS revoked_at ON refresh_tokens TYPE option<datetime>;
DEFINE FIELD IF NOT EXISTS replaced_by_hash ON refresh_tokens TYPE string DEFAULT '';
DEFINE INDEX IF NOT EXISTS refresh_tokens_hash ON refresh_tokens FIELDS token_hash UNIQUE;

-- Password Reset Tokens
DEFINE TABLE IF NOT EXISTS password_resets SCHEMAFULL;
DEFINE FIELD IF NOT EXISTS token_hash ON password_resets TYPE string;
DEFINE FIELD IF NOT EXISTS user_id ON password_resets TYPE string;
DEFINE FIELD IF NOT EXISTS expires_at ON password_resets TYPE datetime;
DEFINE FIELD IF NOT EXISTS created_at ON password_resets TYPE datetime;
DEFINE FIELD IF NOT EXISTS used_at ON password_resets TYPE option<datetime>;
DEFINE INDEX IF NOT EXISTS password_resets_hash ON password_resets FIELDS token_hash UNIQUE;
