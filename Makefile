# Heimdall – common tasks. Run from repo root.

.PHONY: ctl ctl-clean

# Run heimdallctl via Docker. Example: make ctl status | make ctl install | make ctl execution list
ctl:
	./scripts/heimdallctl.sh $(filter-out $@,$(MAKECMDGOALS))

# Remove orphan heimdallctl run containers (stops the "Found orphan containers" warning)
ctl-clean:
	docker compose -f deployments/docker/docker-compose.yml --profile cli down --remove-orphans

# Catch-all so "make ctl status" passes "status" to the script
%:
	@:
