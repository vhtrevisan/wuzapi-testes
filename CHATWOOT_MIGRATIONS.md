# Chatwoot Migrations SQL

Estas são as migrações que serão adicionadas ao `migrations.go`. Estão documentadas aqui separadamente para revisão antes de integração.

## Migration 9: Create Chatwoot Config Table

### PostgreSQL
```sql
-- Migration 9: Create chatwoot_config table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'chatwoot_config') THEN
        CREATE TABLE chatwoot_config (
            user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
            account_id TEXT NOT NULL,
            token TEXT NOT NULL,
            url TEXT NOT NULL,
            inbox_id BIGINT,
            name_inbox TEXT NOT NULL DEFAULT '',
            enabled BOOLEAN DEFAULT FALSE,
            auto_create BOOLEAN DEFAULT FALSE,
            sign_msg BOOLEAN DEFAULT FALSE,
            sign_delimiter TEXT DEFAULT '\n',
            reopen_conversation BOOLEAN DEFAULT FALSE,
            conversation_pending BOOLEAN DEFAULT FALSE,
            merge_brazil_contacts BOOLEAN DEFAULT FALSE,
            organization TEXT DEFAULT '',
            logo TEXT DEFAULT '',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        
        -- Create index for faster lookups
        CREATE INDEX idx_chatwoot_config_enabled ON chatwoot_config (enabled) WHERE enabled = TRUE;
    END IF;
END $$;
```

### SQLite
```sql
CREATE TABLE IF NOT EXISTS chatwoot_config (
    user_id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    token TEXT NOT NULL,
    url TEXT NOT NULL,
    inbox_id INTEGER,
    name_inbox TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN DEFAULT 0,
    auto_create BOOLEAN DEFAULT 0,
    sign_msg BOOLEAN DEFAULT 0,
    sign_delimiter TEXT DEFAULT '\n',
    reopen_conversation BOOLEAN DEFAULT 0,
    conversation_pending BOOLEAN DEFAULT 0,
    merge_brazil_contacts BOOLEAN DEFAULT 0,
    organization TEXT DEFAULT '',
    logo TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_chatwoot_config_enabled ON chatwoot_config (enabled) WHERE enabled = 1;
```

---

## Migration 10: Create Chatwoot Conversations Cache Table

### PostgreSQL
```sql
-- Migration 10: Create chatwoot_conversations table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'chatwoot_conversations') THEN
        CREATE TABLE chatwoot_conversations (
            id SERIAL PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            chat_jid TEXT NOT NULL,
            chatwoot_conversation_id BIGINT NOT NULL,
            chatwoot_contact_id BIGINT NOT NULL,
            chatwoot_inbox_id BIGINT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(user_id, chat_jid)
        );
        
        -- Create indexes for faster lookups
        CREATE INDEX idx_chatwoot_conversations_user ON chatwoot_conversations (user_id);
        CREATE INDEX idx_chatwoot_conversations_chat_jid ON chatwoot_conversations (chat_jid);
        CREATE INDEX idx_chatwoot_conversations_conversation_id ON chatwoot_conversations (chatwoot_conversation_id);
    END IF;
END $$;
```

### SQLite
```sql
CREATE TABLE IF NOT EXISTS chatwoot_conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    chat_jid TEXT NOT NULL,
    chatwoot_conversation_id INTEGER NOT NULL,
    chatwoot_contact_id INTEGER NOT NULL,
    chatwoot_inbox_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, chat_jid),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_chatwoot_conversations_user ON chatwoot_conversations (user_id);
CREATE INDEX IF NOT EXISTS idx_chatwoot_conversations_chat_jid ON chatwoot_conversations (chat_jid);
CREATE INDEX IF NOT EXISTS idx_chatwoot_conversations_conversation_id ON chatwoot_conversations (chatwoot_conversation_id);
```

---

## Migration 11: Create Chatwoot Messages Mapping Table

### PostgreSQL
```sql
-- Migration 11: Create chatwoot_messages table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'chatwoot_messages') THEN
        CREATE TABLE chatwoot_messages (
            id SERIAL PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            message_id TEXT NOT NULL,
            chatwoot_message_id BIGINT NOT NULL,
            chatwoot_conversation_id BIGINT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(user_id, message_id)
        );
        
        -- Create indexes for faster lookups
        CREATE INDEX idx_chatwoot_messages_user ON chatwoot_messages (user_id);
        CREATE INDEX idx_chatwoot_messages_message_id ON chatwoot_messages (message_id);
        CREATE INDEX idx_chatwoot_messages_chatwoot_id ON chatwoot_messages (chatwoot_message_id);
    END IF;
END $$;
```

### SQLite
```sql
CREATE TABLE IF NOT EXISTS chatwoot_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    chatwoot_message_id INTEGER NOT NULL,
    chatwoot_conversation_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, message_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_chatwoot_messages_user ON chatwoot_messages (user_id);
CREATE INDEX IF NOT EXISTS idx_chatwoot_messages_message_id ON chatwoot_messages (message_id);
CREATE INDEX IF NOT EXISTS idx_chatwoot_messages_chatwoot_id ON chatwoot_messages (chatwoot_message_id);
```

---

## Notas de Implementação

### Vantagens da Abordagem com Tabelas Separadas

1. **Normalização:** Evita poluir a tabela `users` com dezenas de colunas específicas do Chatwoot
2. **Performance:** Índices específicos para queries de Chatwoot não impactam queries de usuários
3. **Manutenção:** Facilita adicionar/remover features do Chatwoot sem alterar schema de `users`
4. **Cascata:** `ON DELETE CASCADE` garante limpeza automática ao remover usuário
5. **Modularidade:** Reflete a arquitetura modular do código (`pkg/chatwoot/`)

### Índices Criados

- **chatwoot_config:** Índice parcial em `enabled = TRUE` para acelerar busca de configs ativas
- **chatwoot_conversations:** Índices em `user_id`, `chat_jid` e `chatwoot_conversation_id`
- **chatwoot_messages:** Índices em `user_id`, `message_id` e `chatwoot_message_id`

### Compatibilidade SQLite vs PostgreSQL

- **BOOLEAN:** SQLite usa `0`/`1`, PostgreSQL usa tipo nativo
- **SERIAL:** SQLite usa `INTEGER PRIMARY KEY AUTOINCREMENT`
- **BIGINT:** SQLite usa `INTEGER` (64-bit)
- **TIMESTAMP:** SQLite usa `DATETIME`
- **Foreign Keys:** SQLite precisa de `PRAGMA foreign_keys=ON` (já configurado no Wuzapi)
