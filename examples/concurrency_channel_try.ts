function channelBoolToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const nativeNumberChannel = channel<number>(1);
console.log(channelBoolToNumber(channelTrySend(nativeNumberChannel, 42)));
console.log(channelBoolToNumber(channelTrySend(nativeNumberChannel, 99)));
console.log(channelTryRecvOr(nativeNumberChannel, 7));
console.log(channelTryRecvOr(nativeNumberChannel, 7));
