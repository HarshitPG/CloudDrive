import os
from typing import List, Optional
from fastapi import FastAPI, UploadFile, File, HTTPException
from pydantic import BaseModel
from PyPDF2 import PdfReader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import FAISS
from langchain_ollama import ChatOllama
from langchain_core.prompts import PromptTemplate
from langchain_core.output_parsers import StrOutputParser

app = FastAPI(
    title="Chat with PDFs API (Local LLM)",
    description="An API to upload PDF documents and ask questions using a locally-running Ollama model.",
    version="1.0.0"
)

FAISS_INDEX_PATH = "faiss_index"

# Singletons for preloading
_embeddings: Optional[HuggingFaceEmbeddings] = None
_llm_model: Optional[ChatOllama] = None
_db: Optional[FAISS] = None
_qa_chain = None
_summary_chain = None

# ------------------- Preload models and DB on startup ------------------- #
@app.on_event("startup")
async def startup_event():
    global _embeddings, _llm_model, _db
    print("Starting up: loading embeddings, LLM, and FAISS DB...")

    _embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
    print("Embeddings loaded")

    _llm_model = ChatOllama(model="llama3.2:1b", num_ctx=2048, temperature=0.3)
    try:
        _ = _llm_model.invoke("Warmup")
        print("LLM warmup complete")
    except Exception as e:
        print(f"LLM warmup failed: {e}")

    if os.path.exists(FAISS_INDEX_PATH):
        try:
            _db = FAISS.load_local(FAISS_INDEX_PATH, _embeddings, allow_dangerous_deserialization=True)
            print("FAISS DB loaded")
        except Exception as e:
            print(f"Failed to load FAISS DB: {e}")

# ------------------- Helper functions ------------------- #
def get_pdf_text(pdf_files: List[UploadFile]) -> str:
    text = ""
    for pdf in pdf_files:
        try:
            pdf_reader = PdfReader(pdf.file)
            for page in pdf_reader.pages:
                text += page.extract_text() or ""
        except Exception as e:
            print(f"Error reading {pdf.filename}: {e}")
            continue
    return text

def get_text_chunks(text: str) -> List[str]:
    splitter = RecursiveCharacterTextSplitter(chunk_size=800, chunk_overlap=100, separators=["\n\n", "\n", ".", " "])
    return splitter.split_text(text)

def get_vector_store(text_chunks: List[str]):
    global _db
    if not text_chunks:
        print("No text chunks to process for vector store")
        return
    _db = FAISS.from_texts(text_chunks, _embeddings)
    _db.save_local(FAISS_INDEX_PATH)
    print("Vector store created and saved")

def get_conversational_chain():
    global _qa_chain
    if _qa_chain is None:
        prompt_template = """
        Answer the question as detailed as possible from the provided context.
        If the answer is not in the context, say "answer is not available in the context."

        Context:
        {context}

        Question:
        {question}

        Answer:
        """
        prompt = PromptTemplate(template=prompt_template, input_variables=["context", "question"])
        _qa_chain = prompt | _llm_model | StrOutputParser()
    return _qa_chain

def get_pdf_summary():
    global _summary_chain
    if _summary_chain is None:
        prompt_template = """
        Summarize the PDF as detailed as possible under 250 words.

        Context:
        {context}

        Answer:
        """
        prompt = PromptTemplate(template=prompt_template, input_variables=["context"])
        _summary_chain = prompt | _llm_model | StrOutputParser()
    return _summary_chain

# ------------------- Endpoints ------------------- #
@app.post("/process-pdfs/", summary="Upload and process PDF files")
async def process_pdfs(pdf_files: List[UploadFile] = File(...)):
    if not pdf_files:
        raise HTTPException(status_code=400, detail="No pdf files were uploaded.")

    raw_text = get_pdf_text(pdf_files)
    if not raw_text.strip():
        raise HTTPException(status_code=400, detail="No text could be extracted from PDFs.")

    text_chunks = get_text_chunks(raw_text)
    get_vector_store(text_chunks)

    return {"message": f"Successfully processed {len(pdf_files)} PDF(s). Vector store is ready."}

class QuestionRequest(BaseModel):
    question: str

@app.post("/ask/", summary="Ask a question to the processed PDFs")
async def ask_question(request: QuestionRequest):
    if _db is None:
        raise HTTPException(status_code=404, detail="Vector store not found. Please upload PDFs first.")

    docs = _db.similarity_search(request.question, k=3)
    context = "\n\n".join([d.page_content for d in docs])

    chain = get_conversational_chain()
    response = chain.invoke({"context": context, "question": request.question})
    return {"answer": response}

@app.get("/", summary="Root endpoint")
async def root():
    return {"message": "Welcome to the Chat with PDFs API (Local LLM). Navigate to /docs for API docs."}

@app.post("/summary", summary="Get PDF summary")
async def summary_pdfs(pdf_files: List[UploadFile] = File(...)):
    raw_text = get_pdf_text(pdf_files)
    if not raw_text.strip():
        raise HTTPException(status_code=400, detail="No extractable text in PDF(s).")

    text_chunks = get_text_chunks(raw_text)
    get_vector_store(text_chunks)

    docs = _db.similarity_search("A comprehensive summary of the document's main points and key findings", k=3)
    context = "\n\n".join([d.page_content for d in docs])

    chain = get_pdf_summary()
    response = chain.invoke({"context": context})
    return {"summary": response}
