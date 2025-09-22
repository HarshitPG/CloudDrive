import { motion } from "framer-motion";
import { Star, FileText } from "lucide-react";

export default function StarredView() {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="space-y-6"
    >
      <div>
        <h1 className="text-2xl font-bold text-foreground mb-2">Starred</h1>
        <p className="text-muted-foreground">
          Files and folders you've starred (Coming Soon)
        </p>
      </div>

      <div className="text-center py-12">
        <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center mx-auto mb-4">
          <Star className="w-8 h-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium text-foreground mb-2">
          No starred files
        </h3>
        <p className="text-muted-foreground">Files you star will appear here</p>
      </div>
    </motion.div>
  );
}
