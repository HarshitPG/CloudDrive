import os
import grpc
from concurrent import futures
import time
import logging
from typing import Dict, Optional
from io import BytesIO

import llm_pb2
import llm_pb2_grpc

from PyPDF2 import PdfReader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import FAISS
from langchain_ollama import ChatOllama
from langchain_core.prompts import PromptTemplate
from langchain_core.output_parsers import StrOutputParser

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# -------------------- Configuration --------------------
GRPC_PORT = os.getenv("LLM_GRPC_PORT", "50051")
GRPC_TOKEN = os.getenv("LLM_GRPC_TOKEN", "dev-secret-token")
FAISS_INDEX_BASE = os.getenv("FAISS_INDEX_BASE", "./faiss_indexes")

# -------------------- Global Singletons (aligned with server.py) --------------------
_embeddings: Optional[HuggingFaceEmbeddings] = None
_llm_model: Optional[ChatOllama] = None
_db: Optional[FAISS] = None 
_qa_chain = None
_summary_chain = None
_document_stores: Dict[str, FAISS] = {} 

# -------------------- Initialization (aligned with server.py startup) --------------------
def init_models():
    """Initialize embeddings and LLM on startup (same as server.py)"""
    global _embeddings, _llm_model
    logger.info("Starting up: loading embeddings, LLM, and FAISS DB...")

    _embeddings = HuggingFaceEmbeddings(model_name="all-MiniLM-L6-v2")
    logger.info("Embeddings loaded")

    _llm_model = ChatOllama(model="llama3.2:1b", num_ctx=2048, temperature=0.3)
    try:
        _ = _llm_model.invoke("Warmup")
        logger.info("LLM warmup complete")
    except Exception as e:
        logger.warning(f"LLM warmup failed: {e}")

    os.makedirs(FAISS_INDEX_BASE, exist_ok=True)
    logger.info("Models initialized")

# -------------------- Helper Functions (from server.py) --------------------
def get_pdf_text_from_bytes(content: bytes) -> str:
    """Extract text from PDF bytes (adapted from server.py's get_pdf_text)"""
    text = ""
    try:
        pdf_reader = PdfReader(BytesIO(content))
        for page in pdf_reader.pages:
            text += page.extract_text() or ""
    except Exception as e:
        logger.error(f"Error reading PDF: {e}")
    return text

def get_text_chunks(text: str) -> list:
    """Split text into chunks (same as server.py)"""
    splitter = RecursiveCharacterTextSplitter(
        chunk_size=800,
        chunk_overlap=100,
        separators=["\n\n", "\n", ".", " "]
    )
    return splitter.split_text(text)

def get_vector_store(text_chunks: list):
    """Create and persist global FAISS vector store (same as server.py get_vector_store)"""
    global _db
    if not text_chunks:
        logger.warning("No text chunks to process for vector store")
        return False
    
    _db = FAISS.from_texts(text_chunks, _embeddings)
    logger.info("Vector store created for summary")
    return True

def get_vector_store_for_file(file_id: str, text_chunks: list):
    """Create and persist FAISS vector store for a file (adapted from server.py)"""
    global _document_stores
    if not text_chunks:
        logger.warning("No text chunks to process for vector store")
        return False
    
    _document_stores[file_id] = FAISS.from_texts(text_chunks, _embeddings)
    
    index_path = f"{FAISS_INDEX_BASE}/{file_id}"
    _document_stores[file_id].save_local(index_path)
    logger.info(f"Vector store created and saved for {file_id}")
    return True

def get_conversational_chain():
    """Get Q&A chain (same as server.py)"""
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

def get_pdf_summary_chain():
    """Get summary chain (same as server.py)"""
    global _summary_chain
    if _summary_chain is None:
        prompt_template = """
        Summarize the PDF as detailed as possible under 100 words.
        Context:
        {context}

        Answer:
        """
        prompt = PromptTemplate(template=prompt_template, input_variables=["context"])
        _summary_chain = prompt | _llm_model | StrOutputParser()
    return _summary_chain

def load_vector_store(file_id: str) -> Optional[FAISS]:
    """Load FAISS index from disk if not in memory"""
    if file_id in _document_stores:
        return _document_stores[file_id]
    
    index_path = f"{FAISS_INDEX_BASE}/{file_id}"
    if os.path.exists(index_path):
        try:
            _document_stores[file_id] = FAISS.load_local(
                index_path,
                _embeddings,
                allow_dangerous_deserialization=True
            )
            logger.info(f"Loaded FAISS DB for {file_id} from disk")
            return _document_stores[file_id]
        except Exception as e:
            logger.error(f"Failed to load FAISS DB for {file_id}: {e}")
    
    return None

# -------------------- gRPC Service Implementation --------------------
class LLMServicer(llm_pb2_grpc.LLMServiceServicer):
    
    def _validate_token(self, context):
        """Validate bearer token from metadata"""
        metadata = dict(context.invocation_metadata())
        token = metadata.get("authorization", "")
        if token != f"Bearer {GRPC_TOKEN}":
            context.abort(grpc.StatusCode.UNAUTHENTICATED, "Invalid token")
    
    def Summarize(self, request, context):
        """Generate quick summary (aligned with /summary endpoint in server.py)"""
        self._validate_token(context)
        
        try:
            logger.info(f" Summarizing document: {request.filename}")
            
            raw_text = get_pdf_text_from_bytes(request.content)
            
            if not raw_text.strip():
                return llm_pb2.SummarizeResponse(error="No extractable text in PDF(s).")
            
            text_chunks = get_text_chunks(raw_text)
            if not text_chunks:
                return llm_pb2.SummarizeResponse(error="Failed to chunk text")
            
            success = get_vector_store(text_chunks)
            if not success:
                return llm_pb2.SummarizeResponse(error="Failed to create vector store")
            
            docs = _db.similarity_search(
                "A comprehensive summary of the document's main points and key findings",
                k=5
            )
            context_text = "\n\n".join([d.page_content for d in docs])
            
            chain = get_pdf_summary_chain()
            summary = chain.invoke({"context": context_text})
            
            logger.info(f" Summary generated for {request.filename}")
            return llm_pb2.SummarizeResponse(summary=summary)
            
        except Exception as e:
            logger.error(f"Summarization error: {e}")
            return llm_pb2.SummarizeResponse(error=str(e))
    
    def ProcessDocument(self, request, context):
        """Index full document for chat (aligned with /process-pdfs endpoint)"""
        self._validate_token(context)
        
        try:
            logger.info(f" Processing document {request.file_id}: {request.filename}")
            
            raw_text = get_pdf_text_from_bytes(request.content)
            if not raw_text.strip():
                return llm_pb2.ProcessDocumentResponse(
                    success=False,
                    error="No text could be extracted from PDF"
                )
            
            text_chunks = get_text_chunks(raw_text)
            if not text_chunks:
                return llm_pb2.ProcessDocumentResponse(
                    success=False,
                    error="No text chunks generated"
                )
            
            success = get_vector_store_for_file(request.file_id, text_chunks)
            if not success:
                return llm_pb2.ProcessDocumentResponse(
                    success=False,
                    error="Failed to create vector store"
                )
            
            logger.info(f" Document {request.file_id} indexed successfully")
            return llm_pb2.ProcessDocumentResponse(
                success=True,
                message=f"Successfully processed. Vector store ready with {len(text_chunks)} chunks."
            )
            
        except Exception as e:
            logger.error(f"Processing error: {e}")
            return llm_pb2.ProcessDocumentResponse(success=False, error=str(e))
    
    def Chat(self, request, context):
        """Answer question about indexed document (aligned with /ask endpoint)"""
        self._validate_token(context)
        
        try:
            logger.info(f" Chat request for {request.file_id}: {request.question}")
            db = load_vector_store(request.file_id)

            if db is None:
                return llm_pb2.ChatResponse(
                    error="Vector store not found. Please process the PDF first."
                )
            
            docs = db.similarity_search(request.question, k=3)
            context_text = "\n\n".join([d.page_content for d in docs])

            chain = get_conversational_chain()
            answer = chain.invoke({"context": context_text, "question": request.question})
            
            logger.info(f"Answer generated for {request.file_id}")
            return llm_pb2.ChatResponse(answer=answer)
            
        except Exception as e:
            logger.error(f" Chat error: {e}")
            return llm_pb2.ChatResponse(error=str(e))
    
    def GetDocumentStatus(self, request, context):
        """Check if document is ready for chat"""
        self._validate_token(context)
        
        try:
            if request.file_id in _document_stores:
                return llm_pb2.DocumentStatusResponse(ready=True, status="ready")
            
            index_path = f"{FAISS_INDEX_BASE}/{request.file_id}"
            if os.path.exists(index_path):
                return llm_pb2.DocumentStatusResponse(ready=True, status="ready")
            
            return llm_pb2.DocumentStatusResponse(ready=False, status="not_found")
            
        except Exception as e:
            logger.error(f"Status check error: {e}")
            return llm_pb2.DocumentStatusResponse(
                ready=False,
                status="error",
                error=str(e)
            )

# -------------------- Server Startup --------------------
def serve():
    """Start gRPC server"""
    init_models()
    
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        options=[
            ("grpc.max_send_message_length", 64 * 1024 * 1024), 
            ("grpc.max_receive_message_length", 64 * 1024 * 1024),
        ],
    )
    llm_pb2_grpc.add_LLMServiceServicer_to_server(LLMServicer(), server)
    
    bind_addr = f"0.0.0.0:{GRPC_PORT}"
    server.add_insecure_port(bind_addr)
    
    logger.info(f" gRPC server starting on {bind_addr}")
    logger.info(f" Auth token: {GRPC_TOKEN[:10]}...")
    
    server.start()
    logger.info(" Server ready - listening for requests")
    
    try:
        while True:
            time.sleep(86400)
    except KeyboardInterrupt:
        logger.info("\n Shutting down gracefully...")
        server.stop(0)

if __name__ == "__main__":
    serve()