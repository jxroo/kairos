import { useState, useEffect } from "react";
import { Input } from "@/components/ui/Input";
import { Search } from "lucide-react";

interface MemorySearchProps {
  onSearch: (query: string) => void;
}

export function MemorySearch({ onSearch }: MemorySearchProps) {
  const [value, setValue] = useState("");

  useEffect(() => {
    const t = setTimeout(() => onSearch(value), 300);
    return () => clearTimeout(t);
  }, [value, onSearch]);

  return (
    <div className="relative">
      <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-white/25" />
      <Input
        placeholder="Search memories..."
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="pl-8"
      />
    </div>
  );
}
