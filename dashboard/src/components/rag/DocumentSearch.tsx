import { useState, useEffect } from "react";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Search } from "lucide-react";

interface DocumentSearchProps {
  onSearch: (query: string) => void;
}

export function DocumentSearch({ onSearch }: DocumentSearchProps) {
  const [value, setValue] = useState("");

  useEffect(() => {
    const t = setTimeout(() => onSearch(value), 300);
    return () => clearTimeout(t);
  }, [value, onSearch]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Document Search</CardTitle>
      </CardHeader>
      <div className="relative">
        <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-white/25" />
        <Input
          placeholder="Search indexed documents..."
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="pl-8"
        />
      </div>
    </Card>
  );
}
