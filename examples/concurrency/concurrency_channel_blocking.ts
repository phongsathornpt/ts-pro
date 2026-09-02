const nativeBlockingChannel = channel<number>(1);
channelSend(nativeBlockingChannel, 42);
console.log(channelRecv(nativeBlockingChannel));
