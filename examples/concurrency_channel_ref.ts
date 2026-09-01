function refChannelBoolToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const nativeStringChannel = channel<string>(1);
console.log(refChannelBoolToNumber(channelTrySend(nativeStringChannel, "buffered")));
console.log(channelTryRecvOr(nativeStringChannel, "fallback"));
console.log(channelTryRecvOr(nativeStringChannel, "fallback"));

const nativeAnyChannel = channel<any>(1);
const nativeRefSender = spawn((): void => {
  channelSend(nativeAnyChannel, "task-ref");
});
const nativeRefReceiver = spawn((): any => channelRecv(nativeAnyChannel));
console.log(join(nativeRefReceiver));
join(nativeRefSender);
