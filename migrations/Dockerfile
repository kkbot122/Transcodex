FROM migrate/migrate:4

COPY migrations /migrations

ENTRYPOINT ["/bin/sh", "-c"]
CMD ["migrate -path /migrations -database \"$DATABASE_URL\" up"]
