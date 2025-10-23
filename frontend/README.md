# 🚀 Cloud Drive Frontend

Fast, responsive web app for the Cloud Drive platform. Built with React + TypeScript + Vite, styled with Tailwind, and using Zustand for state and data.

## 🛠️ Tech Stack

- React 19 + TypeScript, Vite 7
- Tailwind CSS 4 + Radix UI
- Zustand (state)
- Axios with token refresh interceptors

## 🧭 Structure (high level)

```
src/
  api/          # REST API clients (auth, files, folders, uploads, llm)
  components/   # UI + feature components (LLM panels, previewer, dialogs)
  hooks/        # useChat, useSummary, use-mobile
  pages/        # Auth pages + Drive views
  stores/       # Zustand stores (auth, drive, llm)
  lib/          # axios, file utils, upload manager
```

## 🔌 Env Vars

The app reads environment variables prefixed with `VITE_`.

- `VITE_API_BASE` (required): Backend base URL. Example: `http://localhost:8080`

Create your env file:

```bash
cp .env.example .env
```

## ⚙️ Setup & Scripts

```bash
# install
npm i   # or npm i / yarn

# dev server
npm dev

# typecheck + build
npm build

```

## ▶️ Run Locally

1. Start backend and infrastructure (see backend README)
2. Configure `.env` with your API base (e.g., `http://localhost:8080`)
3. Start the dev server and open the app
