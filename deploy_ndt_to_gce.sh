#!/bin/bash

set -u
set -x

# These variables should not change much
USAGE="Usage: $0 <project> <vm name> <vm zone>"
PROJECT=${1:?Please provide project name: $USAGE}
GCE_NAME=${2:?Please provide a GCE VM name: $USAGE}
GCE_ZONE=${3:?Please provide GCE zone: $USAGE}
SCP_FILES="certs collectd.prom lame_duck.prom"
IMAGE_TAG="ndt-cloud"
GCE_IMG_PROJECT="cos-cloud"
GCE_IMG_FAMILY="cos-stable"
NDT_DOCKER_IMAGE="pboothe/ndt-cloud:6122c37b15d80b6f84aebd47760fa39e4c570862"

# Set the project and zone for all future gcloud commands.
gcloud config set project $PROJECT
gcloud config set compute/zone $GCE_ZONE

# Make sure that the files we want to copy actually exist.
for scp_file in ${SCP_FILES}; do
  if [[ ! -e "${scp_file}" ]]; then
    echo "Missing required file/dir: ${scp_file}!"
    exit 1
  fi
done

EXISTING_INSTANCE=$(gcloud compute instances list --filter "name=${GCE_NAME}")
if [[ -z "${EXISTING_INSTANCE}" ]]; then
  gcloud compute instances create $GCE_NAME \
    --image-project $GCE_IMG_PROJECT \
    --image-family $GCE_IMG_FAMILY \
    --tags ${IMAGE_TAG} \
    --machine-type n1-standard-4 \
    --boot-disk-size 50GB \
    --metadata-from-file user-data=cloud-config.yaml
fi

# Copy required files to the GCE instance.
gcloud compute scp --recurse $SCP_FILES $GCE_NAME:~

# Build and start a new NDT container, deleting any existing one first.
gcloud compute ssh $GCE_NAME --command \
  '[[ -n "$(docker ps --quiet --filter name=iupui_ndt)" ]] && 
    docker rm --force iupui_ndt'

gcloud compute ssh $GCE_NAME --command "\
  docker run --detach --network host \
    --volume ~/certs:/certs --volume ~/logs:/var/spool/iupui_ndt \
    --restart always --name iupui_ndt ${NDT_DOCKER_IMAGE}"

# Create the directory /var/spool/node_exporter and copy our textfile module
# files into it.
gcloud compute ssh $GCE_NAME --command "sudo mkdir -p /var/spool/node_exporter"
gcloud compute ssh $GCE_NAME --command "\
  sudo cp ~/*.prom /var/spool/node_exporter/"

# Build and start a new Prometheus node_exporter container, deleting any
# existing one first.
gcloud compute ssh $GCE_NAME --command \
  '[[ -n "$(docker ps --quiet --filter name=node_exporter)" ]] &&
    docker rm --force node_exporter'

gcloud compute ssh $GCE_NAME --command "\
  docker run --detach --network host \
    --volume /proc:/host/proc --volume /sys:/host/sys \
    --volume /var/spool/node_exporter:/var/spool/node_exporter \
    --name node_exporter --restart always prom/node-exporter \
    --path.procfs /host/proc --path.sysfs /host/sys \
    --collector.textfile.directory /var/spool/node_exporter \
    --no-collector.arp --no-collector.bcache --no-collector.conntrack \
    --no-collector.edac --no-collector.entropy --no-collector.filefd \
    --no-collector.hwmon --no-collector.infiniband --no-collector.ipvs \
    --no-collector.mdadm --no-collector.netstat --no-collector.sockstat \
    --no-collector.time --no-collector.timex --no-collector.uname \
    --no-collector.vmstat --no-collector.wifi --no-collector.xfs \
    --no-collector.zfs"