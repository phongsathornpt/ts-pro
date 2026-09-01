interface TsnativeConsole {
  log(...values: readonly unknown[]): void;
}

declare const console: TsnativeConsole;

interface TsnativeTask {
  readonly __tsnativeTaskBrand: never;
}

declare function spawn(fn: () => void): TsnativeTask;
declare function join(task: TsnativeTask): void;
declare function yieldNow(): void;
