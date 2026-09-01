interface TsnativeConsole {
  log(...values: readonly unknown[]): void;
}

declare const console: TsnativeConsole;

interface TsnativeTask<T = void> {
  readonly __tsnativeTaskBrand: never;
  readonly __tsnativeTaskResultBrand?: T;
}

declare function spawn<T>(fn: () => T): TsnativeTask<T>;
declare function join<T>(task: TsnativeTask<T>): T;
declare function yieldNow(): void;
