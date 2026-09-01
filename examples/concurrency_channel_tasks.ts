const nativeTaskChannel = channel<number>(0);
const nativeChannelSender = spawn((): void => {
  channelSend(nativeTaskChannel, 42);
});
const nativeChannelReceiver = spawn((): number => channelRecv(nativeTaskChannel));
console.log(join(nativeChannelReceiver));
join(nativeChannelSender);
