FROM ghcr.io/nats-nui/nui:latest

# NUI reads this directory through its --proto-schemas-path option. Keep the
# application contracts and the external schemas they import outside the Buf
# module so local tooling does not affect normal contract generation.
COPY proto/ /protoschemas/default/
COPY infra/nui/proto-schemas/google/ /protoschemas/default/google/
