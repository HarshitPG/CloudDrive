import os
from typing import List
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

def get_pdf_text(pdf_files: List[UploadFile]) -> str:
    """Extracts text from a list of uploaded PDF files."""
    text = ""
    for pdf in pdf_files:
        try:
            pdf_reader = PdfReader(pdf.file)
            for page in pdf_reader.pages:
                text+= page.extract_text() or ""
        except Exception as e:
            print(f"Error reading{pdf.filename}:{e}")
            continue
    return text

def get_text_chunks(text: str) -> List[str]:
    """Splits a long text into smaller, manageable chunks."""
    text_splitter = RecursiveCharacterTextSplitter(chunk_size=1000,chunk_overlap=200)
    chunks = text_splitter.split_text(text)
    return chunks

def get_vector_store(text_chunks: List[str]):
    """Create and saves a FAISS vector store from text chunks."""
    if not text_chunks:
        print("No text chunks to process for vector store")
        return 
    
    embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
    vector_store = FAISS.from_texts(text_chunks, embeddings)
    vector_store.save_local(FAISS_INDEX_PATH)

def get_conversational_chain():
    """Creates a conversational retrieval chain using a local Ollama LLM."""
    prompt_template = """
    Answer the question as detailed as possible from the provided context.
    Make sure to provide all the details. If the answer is not in the provided context,
    just say, "answer is not available in the context." Do not provide a wrong answer.

    Context:
    {context}

    Question:
    {question}

    Answer:
    """
    model = ChatOllama(model="llama3.2")
    prompt = PromptTemplate(template=prompt_template, input_variables=["context", "question"])
    chain = prompt | model | StrOutputParser()
    return chain

def get_pdf_summary():
    prompt_template = """
    Summarize the PDF as detailed as possible from the provided context under 250 words. Make sure to provide all the details. Do not provide a wrong answer.

    Context:
    {context}

    Answer:
    """
    model = ChatOllama(model="llama3.2:1b")
    prompt = PromptTemplate(template=prompt_template, input_variables=["context"])
    chain = prompt | model | StrOutputParser()
    return chain

@app.post("/process-pdfs/", summary="Upload and process PDF files")
async def process_pdfs(pdf_files:List[UploadFile]=File(...)):
    """
    Receives PDF files, extracts text, chunks it, and creates a searchable vector store.
    """
    if not pdf_files:
        raise HTTPException(status_code=400, detail="No pdf files were uploaded.")
    print(f"Received {len(pdf_files)} PDF(s) for processing.")

    raw_text = get_pdf_text(pdf_files)
    if not raw_text.strip():
        raise HTTPException(status_code=400, detail="Could not extract any text from the provided PDF(s).")
    
    text_chunks = get_text_chunks(raw_text)
    get_vector_store(text_chunks)

    print("PDFs processed successfully. Vector store is ready.")
    return {"message": f"Successfully processed {len(pdf_files)} PDF(s). The vector store has been created."}

class QuestionRequest(BaseModel):
    """Request model for the question to be asked."""
    question: str

@app.post("/ask/",summary="Ask a question to the processed PDFs")
async def ask_question(request: QuestionRequest):
    """
    Receives a question, searches the vector store for relevant context,
    and returns an answer from the local language model.
    """
    if not os.path.exists(FAISS_INDEX_PATH):
         raise HTTPException(status_code=404, detail="Vector store not found. Please upload and process your PDF files first via the /process-pdfs/ endpoint.")

    user_question = request.question
    if not user_question:
        raise HTTPException(status_code=400, detail="The 'question' field cannot be empty.")
    
    print(f"Received question: {user_question}")

    embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
    db = FAISS.load_local(FAISS_INDEX_PATH, embeddings, allow_dangerous_deserialization=True)

    docs = db.similarity_search(user_question)
    context = "\n\n".join([doc.page_content for doc in docs])

    chain = get_conversational_chain()
    response = chain.invoke({"context": context, "question": user_question})

    print(f"Generated response: {response}")
    return {"answer": response}

@app.get("/", summary="Root endpoint")
async def root():
    """A simple root endpoint to confirm that the API is running."""
    return {"message": "Welcome to the Chat with PDFs API (Local LLM). Navigate to /docs for the API documentation."}

@app.post("/summary", summary="Get PDF summary from knowledge base")
async def summary_pdfs(pdf_files: List[UploadFile] = File(...)):
    """
    Summarizes a knowledge base. 
    """
    raw_text = get_pdf_text(pdf_files)
    if not raw_text.strip():
        raise HTTPException(status_code=400, detail="The provided PDF is empty or contains no extractable text.")
        
    text_chunks = get_text_chunks(raw_text)
    get_vector_store(text_chunks)
    print("New knowledge base created successfully.")


    print("Loading knowledge base for summarization...")
    embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
    
    try:
        db = FAISS.load_local(FAISS_INDEX_PATH, embeddings, allow_dangerous_deserialization=True)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to load the knowledge base: {e}")

    docs = db.similarity_search("A comprehensive summary of the document's main points and key findings", k=3)
    context = "\n\n".join([d.page_content for d in docs])

    chain = get_pdf_summary() 
    response = chain.invoke({"context": context})

    print(f"Generated summary successfully.")
    return {"summary": response}