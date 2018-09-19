#include <sys/socket.h>
#include <linux/inet_diag.h>
#include <netinet/in.h>
#include <netinet/tcp.h>

#include <errno.h>
#include <stdint.h>
#include <string.h>

int get_bbr_info(int fd, double *bw, double *rtt) {
  if (bw == NULL || rtt == NULL) {
    return EINVAL;  /* You passed me an invalid argument */
  }
  /* With old kernels, like the one that we have by default on Travis, there
     is no support for BBR, which breaks the integration tests. For now we
     just work around it. TODO(bassosimone): Use docker on travis? */
#ifdef TCP_CC_INFO
  union tcp_cc_info ti;
  memset(&ti, 0, sizeof(ti));
  socklen_t tilen = sizeof(ti);
  if (getsockopt(fd, IPPROTO_TCP, TCP_CC_INFO, &ti, &tilen) == -1) {
    return errno;  /* Whatever libc said went wrong */
  }
  /* Apparently, tcp_bbr_info is the only congestion control data structure
     to occupy five 32 bit words. Currently, in September 2018, the other two
     data structures (i.e. Vegas and DCTCP) both occupy four 32 bit words.
     See include/uapi/linux/inet_diag.h in torvalds/linux@bbb6189d. */
  if (tilen != sizeof(struct tcp_bbr_info)) {
    return EINVAL;  /* You passed me a socket that is not using TCP BBR */
  }
  *bw = (double)((((uint64_t)ti.bbr.bbr_bw_hi) << 32) |
                 ((uint64_t)ti.bbr.bbr_bw_lo));
  *rtt = (double)ti.bbr.bbr_min_rtt;
  return 0;
#else
  return ENOSYS;  /* This kernel does not support getting TCP BBR info */
#endif
}
