import { Routes, Route } from "react-router-dom";
import { NavPill } from "@/components/layout/NavPill";
import { ErrorBoundary } from "@/components/ui/ErrorBoundary";
import { SystemStatus } from "@/pages/SystemStatus";
import { MemoryBrowser } from "@/pages/MemoryBrowser";
import { RAGStatus } from "@/pages/RAGStatus";
import { Chat } from "@/pages/Chat";

export default function App() {
  return (
    <div className="min-h-screen bg-bg">
      <NavPill />
      <main className="pt-[72px] px-6 pb-6 max-w-[1440px] mx-auto">
        <ErrorBoundary>
          <Routes>
            <Route path="/" element={<Chat />} />
            <Route path="/system" element={<SystemStatus />} />
            <Route path="/memories" element={<MemoryBrowser />} />
            <Route path="/rag" element={<RAGStatus />} />
          </Routes>
        </ErrorBoundary>
      </main>
    </div>
  );
}
