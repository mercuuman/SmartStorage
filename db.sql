CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TYPE user_status AS ENUM ('active', 'banned', 'deleted');

CREATE TABLE users (
                       id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                       email VARCHAR(255) UNIQUE NOT NULL,
                       password_hash TEXT NOT NULL,
                       name VARCHAR(255),
                       status user_status DEFAULT 'active',
                       created_at TIMESTAMP DEFAULT NOW(),
                       updated_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE sessions (
                          id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                          user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                          refresh_token TEXT NOT NULL,
                          expires_at TIMESTAMP NOT NULL,
                          created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE TABLE organizations (
                               id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                               name VARCHAR(255) NOT NULL,
                               owner_id UUID NOT NULL REFERENCES users(id),
                               plan_id UUID,
                               created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_org_owner ON organizations(owner_id);
CREATE TYPE org_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE organization_members (
                                      id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                                      organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
                                      user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                      role org_role DEFAULT 'member',
                                      joined_at TIMESTAMP DEFAULT NOW(),

                                      UNIQUE (organization_id, user_id)
);
CREATE TABLE plans (
                       id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                       name VARCHAR(50) NOT NULL,
                       max_storage_bytes BIGINT,
                       max_file_size_bytes BIGINT,
                       max_users INT,
                       compression_priority_level INT,
                       deduplication_enabled BOOLEAN DEFAULT true,
                       version_history_days INT,
                       price_monthly NUMERIC(10,2),
                       created_at TIMESTAMP DEFAULT NOW()
);
CREATE TYPE subscription_status AS ENUM ('active', 'canceled', 'expired');

CREATE TABLE subscriptions (
                               id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                               user_id UUID REFERENCES users(id),
                               organization_id UUID REFERENCES organizations(id),
                               plan_id UUID NOT NULL REFERENCES plans(id),
                               status subscription_status DEFAULT 'active',
                               started_at TIMESTAMP DEFAULT NOW(),
                               ends_at TIMESTAMP
);

CREATE INDEX idx_sub_user ON subscriptions(user_id);
CREATE INDEX idx_sub_org ON subscriptions(organization_id);

CREATE TABLE files (
                       id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                       user_id UUID NOT NULL REFERENCES users(id),
                       organization_id UUID REFERENCES organizations(id),
                       filename VARCHAR(255) NOT NULL,
                       current_version_id UUID,
                       is_deleted BOOLEAN DEFAULT false,
                       created_at TIMESTAMP DEFAULT NOW(),
                       updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_files_owner ON files(owner_user_id);
CREATE INDEX idx_files_org ON files(organization_id);
CREATE TABLE file_versions (
                               id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                               file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
                               physical_file_id UUID NOT NULL,
                               version_number INT NOT NULL,
                               uploaded_by UUID REFERENCES users(id),
                               created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_file_versions_file_id ON file_versions(file_id);
CREATE TABLE physical_files (
                                id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                                hash_sha256 VARCHAR(64) UNIQUE NOT NULL,
                                storage_path TEXT NOT NULL,
                                original_size BIGINT NOT NULL,
                                compressed_size BIGINT,
                                compression_algorithm VARCHAR(50),
                                compression_ratio NUMERIC(5,2),
                                reference_count INT DEFAULT 1,
                                created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_physical_hash ON physical_files(hash_sha256);
CREATE TYPE upload_status AS ENUM ('uploading', 'completed', 'failed');

CREATE TABLE upload_sessions (
                                 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                                 user_id UUID NOT NULL REFERENCES users(id),
                                 file_name VARCHAR(255),
                                 status upload_status DEFAULT 'uploading',
                                 temp_path TEXT,
                                 created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE storage_stats (
                               id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                               user_id UUID REFERENCES users(id),
                               org_id UUID REFERENCES organizations(id),
                               total_original_bytes BIGINT,
                               total_compressed_bytes BIGINT,
                               space_saved_bytes BIGINT,
                               compression_efficiency NUMERIC(5,2),
                               calculated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_stats_user ON storage_stats(user_id);
CREATE INDEX idx_stats_org ON storage_stats(org_id);

CREATE TABLE audit_logs (
                            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
                            user_id UUID REFERENCES users(id),
                            action VARCHAR(50) NOT NULL,
                            file_id UUID,
                            ip_address VARCHAR(45),
                            created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_audit_user ON audit_logs(user_id);
CREATE INDEX idx_audit_file ON audit_logs(file_id);

ALTER TABLE organizations
    ADD CONSTRAINT fk_org_plan
        FOREIGN KEY (plan_id) REFERENCES plans(id);


ALTER TABLE files
    ADD CONSTRAINT fk_files_current_version
        FOREIGN KEY (current_version_id) REFERENCES file_versions(id);

ALTER TABLE file_versions
    ADD CONSTRAINT fk_file_versions_physical
        FOREIGN KEY (physical_file_id) REFERENCES physical_files(id);

CREATE INDEX idx_files_owner ON files(owner_user_id);

CREATE INDEX idx_files_owner ON files(user_id);

create table folders
(
    id          uuid default uuid_generate_v4() primary key,
    user_id     uuid not null references users,
    parent_id   uuid references folders on delete cascade,
    name        varchar(255) not null,
    created_at  timestamp default now()
);
alter table files
    add column folder_id uuid references folders;

ALTER TABLE folders ADD COLUMN is_system BOOLEAN DEFAULT FALSE;