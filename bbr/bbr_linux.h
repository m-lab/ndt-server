#ifndef M_LAB_NDT_CLOUD_BBR_BBR_LINUX_H
#define M_LAB_NDT_CLOUD_BBR_BBR_LINUX_H
#ifdef __cplusplus
extern "C" {
#endif

/* get_bbr_info retrieves BBR info from |fd| and stores them in |bw| and
   |rtt| respectively. On success, returns zero. On failure returns a nonzero
   errno value indicating the error that occurred. */
int get_bbr_info(int fd, double *bw, double *rtt);

#ifdef __cplusplus
}  // extern "C"
#endif
#endif M_LAB_NDT_CLOUD_BBR_BBR_LINUX_H
