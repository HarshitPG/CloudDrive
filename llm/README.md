# LLM gRPC Service

Lightweight Python service providing document summarization and Q&A over gRPC. Uses Ollama (llama3.2:1b), HuggingFace embeddings, and FAISS vector stores.

## Tech Stack

- Python 3.11+
- gRPC (grpcio, protobuf)
- LangChain (ChatOllama, Prompt/Parser)
- HuggingFace Embeddings (all-MiniLM-L6-v2)
- FAISS (persistent vector stores per document)
- PyPDF2 (PDF text extraction)
- Ollama runtime (serves local LLMs)

## Ports

- gRPC: 50051 (default; LLM_GRPC_PORT)
- Ollama: 11434 (from docker-compose)

## Auth

- Bearer token via gRPC metadata
- Metadata header: `authorization: Bearer <LLM_GRPC_TOKEN>`

## API Methods

- Summarize(content: bytes, filename: string) -> summary|string error
- ProcessDocument(file_id: string, content: bytes, filename: string) -> success|error
- Chat(file_id: string, question: string) -> answer|error
- GetDocumentStatus(file_id: string) -> ready|status|error

Messages are capped at 64MB (send/receive) to support large PDFs.

## Setup

1. Start Ollama (for the LLM)

```bash
# using provided compose in llm/
docker compose up -d
# pull a model once (inside the container or host with Ollama installed)
docker exec -it ollama ollama pull llama3.2:1b
```

2. Python env

```bash
cd llm
python -m venv venv
# Windows PowerShell: .\\venv\\Scripts\\Activate.ps1
# Windows bash/Git-Bash: source venv/Scripts/activate
# Linux/macOS:
source venv/bin/activate
pip install -r requirements.txt
```

3. Configure env

```bash
cp .env.example .env
# adjust LLM_GRPC_TOKEN if needed
```

4. Run server

```bash
python server.py
```

Server binds to `0.0.0.0:50051` by default.

## Environment

See `.env.example` for full list. Common values:

- `LLM_GRPC_PORT=50051`
- `LLM_GRPC_TOKEN=dev-secret-token`
- `FAISS_INDEX_BASE=./faiss_indexes` (default path)

## Notes

- First requests may be slower due to model warmup and embeddings load.
- Vector stores persist under `faiss_indexes/<file_id>`; safe to keep between runs.
- Ensure the backend uses the same token set in `LLM_GRPC_TOKEN`.
