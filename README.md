# 🚀 Startup Factory

AI-powered startup idea validator using LLM-simulated personas and economic modeling.

![UI Screenshot](https://github.com/adntgv/startup-factory-web/raw/master/screenshot.png)

## Features

- **Hope-Gap Analysis** — Validates if your solution truly meets user hopes
- **100+ AI Personas** — Simulates real-world user behavior
- **Conversion Scoring** — Predicts conversion rates before you build
- **Real-Time Results** — Watch validation run live
- **History Tracking** — Review past validations

## Quick Start

**Local Testing:**
```bash
cd web-ui
node server.js
# Open http://localhost:3737
```

**Deploy to Coolify:**
1. Create new application from GitHub
2. Repository: `adntgv/startup-factory-web`
3. Dockerfile: `web-ui/Dockerfile`
4. Port: `3737`
5. Domain: `factory.adntgv.com`

## How It Works

1. **Input:** Describe your startup idea
2. **Persona Generation:** Creates 100 diverse AI personas matching your target audience
3. **Simulation:** Each persona evaluates your solution against their initial hopes
4. **Scoring:** Combines conversion rate + hope-gap penalty
5. **Results:** See which ideas resonate and why

## Architecture

- **Frontend:** Vanilla JS + Tailwind CSS (zero build step)
- **Backend:** Express.js API calling Go validator binary
- **Validator:** Go 1.22+ with concurrent persona simulation
- **LLM:** Uses MaxClaw (MiniMax M2.5) at maxclaw:9999

## Environment Variables

```bash
GROQ_API_KEY=your_groq_key        # Optional: Groq fallback
CEREBRAS_API_KEY=your_cerebras    # Optional: Cerebras fallback
GITHUB_TOKEN=your_github_token    # Optional: GitHub Models fallback
```

## Project Structure

```
startup-factory/
├── web-ui/               # Web interface
│   ├── index.html       # Single-page app
│   ├── server.js        # Express API
│   └── Dockerfile       # Container build
├── go-validator/         # Core validation engine
│   ├── main.go          # CLI entry
│   ├── pipeline.go      # Validation pipeline
│   ├── profile.go       # Persona generation
│   └── scoring.go       # Hope-gap scoring
└── profile.json          # Validation profile config
```

## Research

- [Hope-Gap Framework](PROFILE_IMPACT_GUIDE.md)
- [Economic Validation](ECONOMIC_VALIDATION.md)
- [Topology Expansion](TOPOLOGY_EXPANSION.md)

## License

MIT

---

**Access:** Currently running at http://localhost:3737 (deploy to factory.adntgv.com pending)
