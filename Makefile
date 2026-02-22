SHELL := /bin/bash

.PHONY: help doctor doctor-strict contracts precommit hook hook-strict

help:
	@echo "FlowForge developer shortcuts:"
	@echo "  make doctor         - run tooling diagnostics (warn profile)"
	@echo "  make doctor-strict  - run tooling diagnostics (strict profile)"
	@echo "  make contracts      - run contract test scripts"
	@echo "  make precommit      - run local pre-commit checks"
	@echo "  make hook           - install managed pre-commit hook"
	@echo "  make hook-strict    - install strict managed pre-commit hook"

doctor:
	./scripts/tooling_doctor.sh

doctor-strict:
	./scripts/tooling_doctor.sh --strict

contracts:
	./scripts/release_checkpoint_contract_test.sh
	./scripts/install_git_hook_contract_test.sh

precommit:
	./scripts/precommit_checks.sh

hook:
	./scripts/install_git_hook.sh

hook-strict:
	./scripts/install_git_hook.sh --strict
