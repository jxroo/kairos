import { useModels } from "@/hooks/useModels";

interface ModelSelectorProps {
  value: string;
  onChange: (model: string) => void;
}

export function ModelSelector({ value, onChange }: ModelSelectorProps) {
  const { data } = useModels();
  const models = data?.data || [];

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="rounded-[10px] bg-white/4 border border-white/10 px-2 py-1 text-xs text-white/65 glass-input transition-colors cursor-pointer"
    >
      <option value="">Auto</option>
      {models.map((m) => (
        <option key={m.id} value={m.id}>
          {m.id}
        </option>
      ))}
    </select>
  );
}
