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

interface TsnativeChannel<T> {
  readonly __tsnativeChannelBrand: never;
  readonly __tsnativeChannelElementBrand?: T;
}

declare function channel<T>(capacity: number): TsnativeChannel<T>;
declare function channelTrySend<T>(channel: TsnativeChannel<T>, value: T): boolean;
declare function channelTryRecvOr<T>(channel: TsnativeChannel<T>, fallback: T): T;

declare function channelSend<T>(channel: TsnativeChannel<T>, value: T): void;
declare function channelRecv<T>(channel: TsnativeChannel<T>): T;
