import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle } from "lucide-react";
import { Button } from "./Button";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Dashboard error:", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <AlertTriangle className="h-10 w-10 text-red-400/70 mb-3" strokeWidth={1.5} />
          <h3 className="text-sm font-medium text-white/65 mb-1">
            Something went wrong
          </h3>
          <p className="text-xs text-white/30 max-w-xs font-mono mb-4">
            {this.state.error.message}
          </p>
          <Button
            variant="secondary"
            onClick={() => this.setState({ error: null })}
          >
            Try again
          </Button>
        </div>
      );
    }
    return this.props.children;
  }
}
