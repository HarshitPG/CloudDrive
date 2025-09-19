import { motion } from "framer-motion";
import { Clock, FileText } from "lucide-react";

export default function RecentView() {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
      className="space-y-6"
    >
      <div>
        <h1 className="text-2xl font-bold text-foreground mb-2">Recent</h1>
        <p className="text-muted-foreground">
          Recently accessed files and folders
        </p>
      </div>

      <div className="text-center py-12">
        <div className="w-16 h-16 bg-muted rounded-full flex items-center justify-center mx-auto mb-4">
          <Clock className="w-8 h-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-medium text-foreground mb-2">
          No recent files
        </h3>
        <p className="text-muted-foreground">
          Recently accessed files will appear here
        </p>
      </div>
    </motion.div>
  );
}
