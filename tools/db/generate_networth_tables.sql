-- Creates the tables behind the Net Worth page.
--   auth.character_tokens     each signed-in character's refresh token, so
--                             snapshots keep running after a restart
--   networth.jita_prices      one sell price per item type per day: the
--                             volume-weighted average of the cheapest 5% of
--                             units listed in Jita 4-4 (PLEX from its global
--                             market), capped at 3x CCP's average
--                             traded price
--   networth.asset_values     what a character's or corporation's assets were
--                             worth each hour, at the latest Jita prices then
--   networth.wallet_balances  a character's wallet, or the total of a
--                             corporation's divisions, each hour
-- Run it against the meadow_works database, e.g.
--   psql -d meadow_works -f tools/db/generate_networth_tables.sql
-- Safe to run more than once.
begin;
create schema if not exists auth;
create schema if not exists networth;

create table if not exists auth.character_tokens (
    character_id bigint primary key,
    character_name text not null,
    refresh_token text not null,
    scopes text[] not null default '{}',
    updated_at timestamptz not null default now()
);

create table if not exists networth.jita_prices (
    price_date date not null,
    type_id bigint not null,
    sell_price numeric(20,2) not null,
    min_price numeric(20,2) not null,
    volume bigint not null,
    order_count int not null,
    fetched_at timestamptz not null default now(),
    primary key (price_date, type_id)
);
create index if not exists jita_prices_type on networth.jita_prices (type_id, price_date desc);

create table if not exists networth.asset_values (
    owner_kind text not null check (owner_kind in ('character', 'corporation')),
    owner_id bigint not null,
    taken_at timestamptz not null,
    value numeric(24,2) not null,
    unpriced_types int not null default 0,
    primary key (owner_kind, owner_id, taken_at)
);

create table if not exists networth.wallet_balances (
    owner_kind text not null check (owner_kind in ('character', 'corporation')),
    owner_id bigint not null,
    taken_at timestamptz not null,
    balance numeric(24,2) not null,
    primary key (owner_kind, owner_id, taken_at)
);
commit;
