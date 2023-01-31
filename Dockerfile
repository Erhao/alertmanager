ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="Moojing-DevOps <xinyu.zhang@moojing.com>"

COPY amtool       /bin/amtool
COPY alertmanager /bin/alertmanager
COPY examples/ha/alertmanager.yml      /etc/alertmanager/alertmanager.ha.example.yml

RUN mkdir -p /alertmanager

USER       root
EXPOSE     9093
VOLUME     [ "/alertmanager" ]
WORKDIR    /alertmanager
ENTRYPOINT [ "/bin/alertmanager" ]
CMD        [ "--config.file=/etc/alertmanager/alertmanager.yml", \
             "--storage.path=/alertmanager", \
             "--data.retention=1440h" ]
