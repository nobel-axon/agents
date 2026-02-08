# Nobel Agents

Chief orchestrator + AI agent roles for the Nobel competitive AI arena.

## Structure

```
agents/
├── chief/          # Orchestrator — the only chain writer
│   ├── cmd/
│   ├── internal/
│   ├── config.testnet.yaml
│   └── Dockerfile
├── agent/          # AI roles (Philosopher, Director, Judge)
│   ├── cmd/
│   ├── internal/
│   ├── docker-compose.yml
│   └── Dockerfile
├── .env.example
└── README.md
```

Two independent Go modules — they don't import each other.

## Chief

The Chief orchestrator is the only service with the operator private key. It creates matches, posts questions, settles winners, and coordinates the AI agent swarm.

```bash
cd chief
go build ./cmd/axon-chief
./axon-chief
```

Port: 9100 (admin: 9101)

## Agent Roles

One binary, role selected by `AGENT_ROLE` env var:

```bash
cd agent
go build ./cmd/axon-agent

AGENT_ROLE=philosopher PORT=9001 ./axon-agent   # Question generation
AGENT_ROLE=director    PORT=9002 ./axon-agent   # Personality creation
AGENT_ROLE=judge       PORT=9003 ./axon-agent   # Answer evaluation (run 3x)
```

| Role | Port | Description |
|------|------|-------------|
| Philosopher | 9001 | Generates Nobel Inquiries via LLM failover |
| Director | 9002 | Creates unique judge personalities per match |
| Judge | 9003-9005 | Evaluates answers with assigned personality |

## LLM Failover

All AI roles use a three-provider failover chain: GLM (Zhipu) → Kimi (Moonshot) → Claude.

## Build & Test

```bash
cd chief && go build ./cmd/axon-chief && go test ./...
cd agent && go build ./cmd/axon-agent && go test ./...
```
