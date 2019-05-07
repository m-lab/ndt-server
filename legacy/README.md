# legacy ndt-server code

All code in this directory tree is related to the support of the legacy NDT
protocol. We have many extant clients that use this protocol, and we don't
want to leave them high and dry, but new clients are encouraged to use the
services provided by ndt7. The test is streamlined, the client is easier to
write, and basically everything about it is better.

In this subtree, we support existing clients, but we will be adding no new
functionality. If you are reading this and trying to decide how to implement
a speed test, use ndt7 and not the legacy protocol. The legacy protocol is
deprecated. It will be supported until usage drops to very low levels, but it
is also not recommended for new integrations or code.
