package types

// AccessModifier represents the visibility level of class members
type AccessModifier int

const (
	AccessPublic AccessModifier = iota
	AccessPrivate
	AccessProtected
)

// String returns the string representation of the access modifier
func (a AccessModifier) String() string {
	switch a {
	case AccessPublic:
		return "public"
	case AccessPrivate:
		return "private"
	case AccessProtected:
		return "protected"
	default:
		return "unknown"
	}
}

// MemberAccessInfo contains access control information for a class member
type MemberAccessInfo struct {
	AccessLevel AccessModifier
	IsStatic    bool
	IsReadonly  bool
	IsGetter    bool  // This property is defined with 'get' keyword
	IsSetter    bool  // This property is defined with 'set' keyword
}

// ClassMetadata contains class-specific type information for access control
type ClassMetadata struct {
	// Name of the class this type represents
	ClassName string
	
	// Access control information for each member
	MemberAccess map[string]*MemberAccessInfo
	
	// Indicates this is a class instance type (not the constructor)
	IsClassInstance bool
	
	// Indicates this is a class constructor type
	IsClassConstructor bool
	
	// Reference to the source class declaration (if available)
	// This is used for inheritance checks and access validation
	SourceClassName string
	
	// Inheritance relationships
	SuperClassName string     // The class this class extends (if any)
	SuperConstructorType Type // The resolved constructor type of the superclass (if any)
	ImplementedInterfaces []string // The interfaces this class implements
}

// NewClassMetadata creates a new ClassMetadata instance
func NewClassMetadata(className string, isInstance bool) *ClassMetadata {
	return &ClassMetadata{
		ClassName:             className,
		MemberAccess:          make(map[string]*MemberAccessInfo),
		IsClassInstance:       isInstance,
		IsClassConstructor:    !isInstance,
		SourceClassName:       className,
		SuperClassName:        "", // No inheritance by default
		SuperConstructorType:  nil, // No inheritance by default
		ImplementedInterfaces: []string{}, // No interfaces by default
	}
}

// AddMember adds access control information for a class member
func (cm *ClassMetadata) AddMember(memberName string, accessLevel AccessModifier, isStatic, isReadonly bool) {
	cm.MemberAccess[memberName] = &MemberAccessInfo{
		AccessLevel: accessLevel,
		IsStatic:    isStatic,
		IsReadonly:  isReadonly,
		IsGetter:    false,
		IsSetter:    false,
	}
}

// AddGetterMember adds a getter method with access control information
func (cm *ClassMetadata) AddGetterMember(memberName string, accessLevel AccessModifier, isStatic bool) {
	cm.MemberAccess[memberName] = &MemberAccessInfo{
		AccessLevel: accessLevel,
		IsStatic:    isStatic,
		IsReadonly:  false, // Getters are not readonly in the traditional sense
		IsGetter:    true,
		IsSetter:    false,
	}
}

// AddSetterMember adds a setter method with access control information
func (cm *ClassMetadata) AddSetterMember(memberName string, accessLevel AccessModifier, isStatic bool) {
	cm.MemberAccess[memberName] = &MemberAccessInfo{
		AccessLevel: accessLevel,
		IsStatic:    isStatic,
		IsReadonly:  false,
		IsGetter:    false,
		IsSetter:    true,
	}
}

// GetMemberAccess returns the access information for a member, or nil if not found
func (cm *ClassMetadata) GetMemberAccess(memberName string) *MemberAccessInfo {
	return cm.MemberAccess[memberName]
}

// HasMember checks if a member exists in this class
func (cm *ClassMetadata) HasMember(memberName string) bool {
	_, exists := cm.MemberAccess[memberName]
	return exists
}

// SetSuperClass sets the superclass for this class
func (cm *ClassMetadata) SetSuperClass(superClassName string) {
	cm.SuperClassName = superClassName
}

// SetSuperClassWithConstructor sets both the superclass name and its resolved constructor type
func (cm *ClassMetadata) SetSuperClassWithConstructor(superClassName string, constructorType Type) {
	cm.SuperClassName = superClassName
	cm.SuperConstructorType = constructorType
}

// AddImplementedInterface adds an interface that this class implements
func (cm *ClassMetadata) AddImplementedInterface(interfaceName string) {
	cm.ImplementedInterfaces = append(cm.ImplementedInterfaces, interfaceName)
}

// ExtendsClass returns true if this class extends the given class name
func (cm *ClassMetadata) ExtendsClass(className string) bool {
	return cm.SuperClassName == className
}

// ImplementsInterface returns true if this class implements the given interface
func (cm *ClassMetadata) ImplementsInterface(interfaceName string) bool {
	for _, impl := range cm.ImplementedInterfaces {
		if impl == interfaceName {
			return true
		}
	}
	return false
}

// IsSubclassOf returns true if this class is a subclass of the given class name
// This checks the entire inheritance chain, not just direct inheritance
func (cm *ClassMetadata) IsSubclassOf(targetClass string, getClassMeta func(string) *ClassMetadata) bool {
	if cm.SuperClassName == "" {
		return false // No superclass
	}
	
	if cm.SuperClassName == targetClass {
		return true // Direct inheritance
	}
	
	// Check if superclass is a subclass of the target (recursive)
	if getClassMeta != nil {
		superMeta := getClassMeta(cm.SuperClassName)
		if superMeta != nil {
			return superMeta.IsSubclassOf(targetClass, getClassMeta)
		}
	}
	
	return false
}

// IsAccessibleFrom checks if a member is accessible from a given class context
func (cm *ClassMetadata) IsAccessibleFrom(memberName string, accessContext *AccessContext) bool {
	memberInfo := cm.GetMemberAccess(memberName)
	if memberInfo == nil {
		return false // Member doesn't exist
	}
	
	switch memberInfo.AccessLevel {
	case AccessPublic:
		return true // Always accessible
		
	case AccessPrivate:
		// Only accessible within the same class
		return accessContext != nil && 
			   accessContext.CurrentClassName == cm.SourceClassName
			   
	case AccessProtected:
		// Accessible within the same class or subclasses
		return accessContext != nil && 
			   (accessContext.CurrentClassName == cm.SourceClassName ||
			    accessContext.IsSubclassOf(cm.SourceClassName))
	}
	
	return false
}

// AccessContext represents the context from which a member is being accessed
type AccessContext struct {
	// Name of the class currently being checked
	CurrentClassName string
	
	// Type of access context
	ContextType AccessContextType
	
	// Whether we're inside a constructor
	IsInConstructor bool
	
	// Whether we're in a static context
	IsStaticContext bool
	
	// Function to check inheritance relationships
	// Returns true if currentClass is a subclass of targetClass
	IsSubclassOfFunc func(currentClass, targetClass string) bool
}

// AccessContextType represents where the access is happening from
type AccessContextType int

const (
	AccessContextExternal AccessContextType = iota
	AccessContextInstanceMethod
	AccessContextStaticMethod
	AccessContextConstructor
)

// String returns the string representation of the access context type
func (act AccessContextType) String() string {
	switch act {
	case AccessContextExternal:
		return "external"
	case AccessContextInstanceMethod:
		return "instance method"
	case AccessContextStaticMethod:
		return "static method"
	case AccessContextConstructor:
		return "constructor"
	default:
		return "unknown"
	}
}

// IsSubclassOf checks if the current class is a subclass of the target class
func (ac *AccessContext) IsSubclassOf(targetClass string) bool {
	if ac.IsSubclassOfFunc == nil {
		return false // No inheritance checking available
	}
	return ac.IsSubclassOfFunc(ac.CurrentClassName, targetClass)
}

// NewAccessContext creates a new access context
func NewAccessContext(className string, contextType AccessContextType) *AccessContext {
	return &AccessContext{
		CurrentClassName: className,
		ContextType:      contextType,
		IsInConstructor:  contextType == AccessContextConstructor,
		IsStaticContext:  contextType == AccessContextStaticMethod,
	}
}