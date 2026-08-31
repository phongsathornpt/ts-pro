interface TsnativeConsole {
  log(...values: readonly unknown[]): void;
}

declare const console: TsnativeConsole;
