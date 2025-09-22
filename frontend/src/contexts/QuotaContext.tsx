import {
  createContext,
  useContext,
  useState,
  useCallback,
  ReactNode,
} from "react";
import { fetchQuota, type QuotaResponse } from "../api/quota";

interface QuotaContextType {
  quota: QuotaResponse | null;
  refreshQuota: () => Promise<void>;
  isLoading: boolean;
}

const QuotaContext = createContext<QuotaContextType | undefined>(undefined);

export const useQuota = () => {
  const context = useContext(QuotaContext);
  if (context === undefined) {
    throw new Error("useQuota must be used within a QuotaProvider");
  }
  return context;
};

interface QuotaProviderProps {
  children: ReactNode;
}

export const QuotaProvider: React.FC<QuotaProviderProps> = ({ children }) => {
  const [quota, setQuota] = useState<QuotaResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const refreshQuota = useCallback(async () => {
    setIsLoading(true);
    try {
      const quotaData = await fetchQuota();
      setQuota(quotaData);
      console.log("Quota data updated:", quotaData);
    } catch (error) {
      console.error("Failed to fetch quota:", error);
      setQuota({
        original_bytes: 0,
        deduped_bytes: 0,
        quota_bytes: 1,
        savings_bytes: 0,
        savings_percent: 0,
        quota_used_percent: 0,
      });
    } finally {
      setIsLoading(false);
    }
  }, []);

  return (
    <QuotaContext.Provider value={{ quota, refreshQuota, isLoading }}>
      {children}
    </QuotaContext.Provider>
  );
};
