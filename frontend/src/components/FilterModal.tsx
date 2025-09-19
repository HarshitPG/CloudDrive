import React, { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Filter, X } from "lucide-react";
import type { SearchFilters } from "@/api/search";

const MIME_TYPES = [
  { value: "all", label: "All types" },
  { value: "image/", label: "Images" },
  { value: "video/", label: "Videos" },
  { value: "audio/", label: "Audio" },
  { value: "application/pdf", label: "PDF" },
  { value: "application/msword", label: "Word Documents" },
  { value: "application/vnd.ms-excel", label: "Excel Files" },
  { value: "application/vnd.ms-powerpoint", label: "PowerPoint" },
  { value: "text/", label: "Text Files" },
  { value: "application/zip", label: "Archives" },
];

const SORT_OPTIONS = [
  { value: "created_at_desc", label: "Newest first" },
  { value: "created_at_asc", label: "Oldest first" },
  { value: "updated_at_desc", label: "Recently updated" },
  { value: "updated_at_asc", label: "Least recently updated" },
  { value: "filename_asc", label: "Name A-Z" },
  { value: "filename_desc", label: "Name Z-A" },
  { value: "size_desc", label: "Largest first" },
  { value: "size_asc", label: "Smallest first" },
];

const SIZE_PRESETS = [
  { label: "Small (< 1MB)", minSize: undefined, maxSize: 1024 * 1024 },
  { label: "Medium (1-10MB)", minSize: 1024 * 1024, maxSize: 10 * 1024 * 1024 },
  {
    label: "Large (10-100MB)",
    minSize: 10 * 1024 * 1024,
    maxSize: 100 * 1024 * 1024,
  },
  {
    label: "Very Large (> 100MB)",
    minSize: 100 * 1024 * 1024,
    maxSize: undefined,
  },
];

type FilterModalProps = {
  filters: SearchFilters;
  onFiltersChange: (filters: SearchFilters) => void;
  onReset: () => void;
};

export default function FilterModal({
  filters,
  onFiltersChange,
  onReset,
}: FilterModalProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [localFilters, setLocalFilters] = useState<SearchFilters>(filters);
  const [tagInput, setTagInput] = useState("");

  const handleApply = () => {
    onFiltersChange(localFilters);
    setIsOpen(false);
  };

  const handleReset = () => {
    const resetFilters = { q: filters.q };
    setLocalFilters(resetFilters);
    onReset();
    setIsOpen(false);
  };

  const addTag = () => {
    if (tagInput.trim() && !localFilters.tags?.includes(tagInput.trim())) {
      setLocalFilters({
        ...localFilters,
        tags: [...(localFilters.tags || []), tagInput.trim()],
      });
      setTagInput("");
    }
  };

  const removeTag = (tagToRemove: string) => {
    setLocalFilters({
      ...localFilters,
      tags: localFilters.tags?.filter((tag) => tag !== tagToRemove),
    });
  };

  const setSizePreset = (preset: (typeof SIZE_PRESETS)[0]) => {
    setLocalFilters({
      ...localFilters,
      minSize: preset.minSize,
      maxSize: preset.maxSize,
    });
  };

  const formatBytes = (bytes?: number) => {
    if (!bytes) return "";
    const sizes = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + " " + sizes[i];
  };

  const hasActiveFilters = () => {
    return !!(
      localFilters.mime ||
      localFilters.minSize ||
      localFilters.maxSize ||
      localFilters.dateFrom ||
      localFilters.dateTo ||
      (localFilters.tags && localFilters.tags.length > 0) ||
      localFilters.uploader ||
      localFilters.folderId ||
      (localFilters.sort && localFilters.sort !== "created_at_desc")
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="relative">
          <Filter className="w-4 h-4 mr-2" />
          Filter
          {hasActiveFilters() && (
            <div className="absolute -top-1 -right-1 w-2 h-2 bg-primary rounded-full" />
          )}
        </Button>
      </DialogTrigger>

      <DialogContent className="max-w-md max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Filter Files</DialogTitle>
          <DialogDescription>
            Apply filters to search and organize your files
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          {/* File Type */}
          <div className="space-y-2">
            <Label>File Type</Label>
            <Select
              value={localFilters.mime || "all"}
              onValueChange={(value) =>
                setLocalFilters({
                  ...localFilters,
                  mime: value === "all" ? undefined : value,
                })
              }
            >
              <SelectTrigger>
                <SelectValue placeholder="All types" />
              </SelectTrigger>
              <SelectContent>
                {MIME_TYPES.map((type) => (
                  <SelectItem key={type.value} value={type.value}>
                    {type.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* File Size */}
          <div className="space-y-3">
            <Label>File Size</Label>

            {/* Size Presets */}
            <div className="grid grid-cols-2 gap-2">
              {SIZE_PRESETS.map((preset, index) => (
                <Button
                  key={index}
                  variant="outline"
                  size="sm"
                  onClick={() => setSizePreset(preset)}
                  className="text-xs h-8"
                >
                  {preset.label}
                </Button>
              ))}
            </div>

            {/* Custom Size Range */}
            <div className="space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <Label className="text-xs">Min Size (bytes)</Label>
                  <Input
                    type="number"
                    placeholder="0"
                    value={localFilters.minSize || ""}
                    onChange={(e) =>
                      setLocalFilters({
                        ...localFilters,
                        minSize: e.target.value
                          ? parseInt(e.target.value)
                          : undefined,
                      })
                    }
                  />
                </div>
                <div>
                  <Label className="text-xs">Max Size (bytes)</Label>
                  <Input
                    type="number"
                    placeholder="No limit"
                    value={localFilters.maxSize || ""}
                    onChange={(e) =>
                      setLocalFilters({
                        ...localFilters,
                        maxSize: e.target.value
                          ? parseInt(e.target.value)
                          : undefined,
                      })
                    }
                  />
                </div>
              </div>
              {(localFilters.minSize || localFilters.maxSize) && (
                <div className="text-xs text-muted-foreground">
                  Range: {formatBytes(localFilters.minSize) || "0 B"} -{" "}
                  {formatBytes(localFilters.maxSize) || "∞"}
                </div>
              )}
            </div>
          </div>

          {/* Date Range */}
          <div className="space-y-2">
            <Label>Date Range</Label>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <Label className="text-xs">From</Label>
                <Input
                  type="date"
                  value={
                    localFilters.dateFrom
                      ? localFilters.dateFrom.split("T")[0]
                      : ""
                  }
                  onChange={(e) =>
                    setLocalFilters({
                      ...localFilters,
                      dateFrom: e.target.value
                        ? `${e.target.value}T00:00:00Z`
                        : undefined,
                    })
                  }
                />
              </div>
              <div>
                <Label className="text-xs">To</Label>
                <Input
                  type="date"
                  value={
                    localFilters.dateTo ? localFilters.dateTo.split("T")[0] : ""
                  }
                  onChange={(e) =>
                    setLocalFilters({
                      ...localFilters,
                      dateTo: e.target.value
                        ? `${e.target.value}T23:59:59Z`
                        : undefined,
                    })
                  }
                />
              </div>
            </div>
          </div>

          {/* Tags */}
          <div className="space-y-2">
            <Label>Tags</Label>
            <div className="flex gap-2">
              <Input
                placeholder="Add tag..."
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addTag();
                  }
                }}
              />
              <Button onClick={addTag} size="sm" disabled={!tagInput.trim()}>
                Add
              </Button>
            </div>
            {localFilters.tags && localFilters.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {localFilters.tags.map((tag) => (
                  <Badge key={tag} variant="secondary" className="text-xs">
                    {tag}
                    <button
                      onClick={() => removeTag(tag)}
                      className="ml-1 hover:text-destructive"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </Badge>
                ))}
              </div>
            )}
          </div>

          {/* Sort Order */}
          <div className="space-y-2">
            <Label>Sort By</Label>
            <Select
              value={localFilters.sort || "created_at_desc"}
              onValueChange={(value) =>
                setLocalFilters({ ...localFilters, sort: value })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SORT_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Uploader */}
          <div className="space-y-2">
            <Label>Uploader (Username)</Label>
            <Input
              placeholder="Filter by uploader..."
              value={localFilters.uploader || ""}
              onChange={(e) =>
                setLocalFilters({
                  ...localFilters,
                  uploader: e.target.value || undefined,
                })
              }
            />
          </div>
        </div>

        {/* Actions */}
        <div className="flex gap-2 pt-4">
          <Button onClick={handleReset} variant="outline" className="flex-1">
            Reset
          </Button>
          <Button onClick={handleApply} className="flex-1">
            Apply Filters
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
