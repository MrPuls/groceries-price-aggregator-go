-- Stores (Shop)
create table if not exists stores (
                                      id bigserial primary key,
                                      code text not null unique,
                                      name text not null
);

-- Products as exposed by a store
create table if not exists products (
                                        id bigserial primary key,
                                        store_id bigint not null references stores(id) on delete cascade,
                                        ref text not null,
                                        name text not null,
                                        url text,
                                        created_at timestamptz not null default now(),
                                        updated_at timestamptz not null default now(),
                                        unique (store_id, ref)
);

-- Categories per store
create table if not exists categories (
                                          id bigserial primary key,
                                          store_id bigint not null references stores(id) on delete cascade,
                                          slug text not null,
                                          name text default null,
                                          created_at timestamptz not null default now(),
                                          updated_at timestamptz not null default now(),
                                          unique (store_id, slug)
);

-- Price time series (numeric money, currency stored explicitly)
create table if not exists prices (
                                      id bigserial primary key,
                                      product_id bigint not null references products(id) on delete cascade,
                                      price text not null,
                                      currency text not null default 'UAH',
                                      created_at timestamptz not null default now(),
                                      updated_at timestamptz not null default now()
);

-- Seed common stores you scrape (safe if re-run)
insert into stores(code, name) values
                                   ('atb', 'ATB'),
                                   ('varus', 'Varus'),
                                   ('metro', 'Metro'),
                                   ('silpo', 'Silpo')
on conflict (code) do nothing;

CREATE TEXT SEARCH DICTIONARY ukrainian_hunspell (
    TEMPLATE = ispell,
    DictFile = ukrainian,
    AffFile = ukrainian,
    Stopwords = ukrainian
    );

CREATE TEXT SEARCH CONFIGURATION ukrainian (COPY = english);

ALTER TEXT SEARCH CONFIGURATION ukrainian
    ALTER MAPPING FOR asciiword, asciihword, hword_asciipart,
          word, hword, hword_part
              WITH ukrainian_hunspell, simple;