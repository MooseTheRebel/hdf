export namespace cli {

	export class ConfigInfo {
	    path: string;
	    exists: boolean;
	    content: string;

	    static createFrom(source: any = {}) {
	        return new ConfigInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.content = source["content"];
	    }
	}
	export class DivergedFile {
	    path: string;
	    diff: string;

	    static createFrom(source: any = {}) {
	        return new DivergedFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.diff = source["diff"];
	    }
	}
	export class EnrollResult {
	    message: string;

	    static createFrom(source: any = {}) {
	        return new EnrollResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	    }
	}
	export class EnrollStartInfo {
	    path: string;
	    isNewFile: boolean;
	    diff: string;

	    static createFrom(source: any = {}) {
	        return new EnrollStartInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.isNewFile = source["isNewFile"];
	        this.diff = source["diff"];
	    }
	}
	export class FileStatus {
	    path: string;
	    status: string;

	    static createFrom(source: any = {}) {
	        return new FileStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	    }
	}
	export class IncomingFile {
	    path: string;
	    diff: string;

	    static createFrom(source: any = {}) {
	        return new IncomingFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.diff = source["diff"];
	    }
	}
	export class InitBranchCollision {
	    branch: string;

	    static createFrom(source: any = {}) {
	        return new InitBranchCollision(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.branch = source["branch"];
	    }
	}
	export class InitResult {
	    message: string;

	    static createFrom(source: any = {}) {
	        return new InitResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	    }
	}
	export class InitStartInfo {
	    collision?: InitBranchCollision;

	    static createFrom(source: any = {}) {
	        return new InitStartInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collision = this.convertValues(source["collision"], InitBranchCollision);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkStartInfo {
	    message: string;
	    incomingFiles: IncomingFile[];

	    static createFrom(source: any = {}) {
	        return new LinkStartInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.incomingFiles = this.convertValues(source["incomingFiles"], IncomingFile);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkedFile {
	    path: string;
	    error: string;

	    static createFrom(source: any = {}) {
	        return new LinkedFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.error = source["error"];
	    }
	}
	export class PreservedFile {
	    path: string;

	    static createFrom(source: any = {}) {
	        return new PreservedFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class PromoteResult {
	    message: string;

	    static createFrom(source: any = {}) {
	        return new PromoteResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	    }
	}
	export class PromoteStartInfo {
	    preserved: PreservedFile[];
	    diverged: DivergedFile[];

	    static createFrom(source: any = {}) {
	        return new PromoteStartInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preserved = this.convertValues(source["preserved"], PreservedFile);
	        this.diverged = this.convertValues(source["diverged"], DivergedFile);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReportIssueResult {
	    path: string;

	    static createFrom(source: any = {}) {
	        return new ReportIssueResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	    }
	}
	export class StatusInfo {
	    git_push_target: string;
	    local_dotfiles_dir: string;
	    branch: string;
	    last_commit: string;
	    last_sync: string;
	    files: FileStatus[];

	    static createFrom(source: any = {}) {
	        return new StatusInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.git_push_target = source["git_push_target"];
	        this.local_dotfiles_dir = source["local_dotfiles_dir"];
	        this.branch = source["branch"];
	        this.last_commit = source["last_commit"];
	        this.last_sync = source["last_sync"];
	        this.files = this.convertValues(source["files"], FileStatus);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
