import { Component, type ReactNode } from "react";

// Model output is arbitrary text and the renderers that read it can throw —
// KaTeX does it for one undefined control sequence. A throw during render
// unmounts the whole tree, so the window goes blank over a single bad token.
// Falling one message back to its source text keeps the rest readable.
export class Boundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
