create extension if not exists pgcrypto;

create type subscription_status as enum (
  'incomplete',
  'trialing',
  'active',
  'past_due',
  'canceled',
  'unpaid'
);

create type usage_event_type as enum (
  'trial_grant',
  'translation_seconds',
  'manual_adjustment'
);

create table users (
  id uuid primary key default gen_random_uuid(),
  auth_subject text not null unique,
  email text not null unique,
  display_name text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table plans (
  id text primary key,
  name text not null,
  weekly_limit_seconds integer not null check (weekly_limit_seconds > 0),
  stripe_price_id text unique,
  created_at timestamptz not null default now()
);

create table subscriptions (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  plan_id text not null references plans(id),
  stripe_customer_id text not null,
  stripe_subscription_id text not null unique,
  status subscription_status not null,
  current_period_start timestamptz,
  current_period_end timestamptz,
  cancel_at_period_end boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table trial_grants (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  limit_seconds integer not null check (limit_seconds > 0),
  starts_at timestamptz not null,
  ends_at timestamptz not null,
  created_at timestamptz not null default now(),
  check (ends_at > starts_at)
);

create table quota_periods (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  plan_id text not null references plans(id),
  period_start timestamptz not null,
  period_end timestamptz not null,
  limit_seconds integer not null check (limit_seconds > 0),
  used_seconds integer not null default 0 check (used_seconds >= 0),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (user_id, period_start),
  check (period_end > period_start)
);

create table translation_sessions (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  source_language text not null,
  target_language text not null,
  input_mode text not null,
  status text not null default 'created',
  started_at timestamptz not null default now(),
  ended_at timestamptz,
  created_at timestamptz not null default now()
);

create table usage_events (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  quota_period_id uuid references quota_periods(id) on delete set null,
  translation_session_id uuid references translation_sessions(id) on delete set null,
  event_type usage_event_type not null,
  seconds integer not null,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create index usage_events_user_created_idx on usage_events(user_id, created_at desc);
create index translation_sessions_user_created_idx on translation_sessions(user_id, created_at desc);

insert into plans (id, name, weekly_limit_seconds)
values
  ('trial', 'Trial', 900),
  ('premium', 'Premium', 18000);
