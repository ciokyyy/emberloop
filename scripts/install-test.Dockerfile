# Fresh-machine test of install.sh. Build from the repo root:
#   docker build -f scripts/install-test.Dockerfile -t el-install-test .
#   docker run --rm -t el-install-test
# -t gives `el deps` a terminal, so it runs sudo apt-get itself.
FROM golang:1.26-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends sudo unzip \
    && useradd -m tester && echo 'tester ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/tester
USER tester
WORKDIR /home/tester/el
COPY --chown=tester . .
ENV PATH="/home/tester/.local/bin:/home/tester/.bun/bin:${PATH}"
# Package lists stay so `deps` can apt-get install on a fresh image.
CMD ["./install.sh", "--yes"]
