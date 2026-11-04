package types

import "strings"

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
	IsGetter    bool // This property is defined with 'get' keyword
	IsSetter    bool // This property is defined with 'set' keyword
	IsMethod    bool // This member is a method declaration
	// SetterAccessLevel is a get/set pair's setter accessibility when it
	// differs from the getter's (AccessLevel): reads check the getter's,
	// writes the setter's (tsc's getDeclarationModifierFlagsFromSymbol).
	SetterAccessLevel AccessModifier
	HasSetterAccess   bool
	// IsField: a property declaration of the class body (not an auto
	// accessor, parameter property or merged interface member), which lives
	// on the instance and so is not reachable through super (TS2855).
	IsField bool
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

	// ConstructorAccess is the declared accessibility of a class
	// constructor type's constructor (`private constructor()`).
	ConstructorAccess AccessModifier

	// DeclFile and DeclStart/DeclEnd locate the class body, for checks
	// that allow what lexically sits inside the class declaration.
	DeclFile           string
	DeclStart, DeclEnd int

	// DisplayText is how the class's declared type prints (`D<T>` for a
	// generic class); empty means ClassName.
	DisplayText string

	// DeclPos is where the class was declared (file and offset), recorded
	// on its instance type: unions print classes in declaration order.
	DeclPosFile string
	DeclPos     int

	// Reference to the source class declaration (if available)
	// This is used for inheritance checks and access validation
	SourceClassName string

	// Inheritance relationships
	HasExtendsClause      bool     // The declaration has an `extends` clause (even if it could not be resolved)
	ExtendsNull           bool     // The extends clause is the literal `null`
	SuperClassName        string   // The class this class extends (if any)
	SuperConstructorType  Type     // The resolved constructor type of the superclass (if any)
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
		SuperClassName:        "",         // No inheritance by default
		SuperConstructorType:  nil,        // No inheritance by default
		ImplementedInterfaces: []string{}, // No interfaces by default
	}
}

// AddMember adds access control information for a class member
func (cm *ClassMetadata) AddMember(memberName string, accessLevel AccessModifier, isStatic, isReadonly bool) {
	accessLevel = privateNameAccess(memberName, accessLevel)
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
	accessLevel = privateNameAccess(memberName, accessLevel)
	// A setter of the same name already declared makes this a get/set pair.
	hasSetter := false
	var setterAccess AccessModifier
	if prev := cm.MemberAccess[memberName]; prev != nil && prev.IsSetter && prev.IsStatic == isStatic {
		hasSetter = true
		setterAccess = prev.AccessLevel
		if prev.HasSetterAccess {
			setterAccess = prev.SetterAccessLevel
		}
	}
	cm.MemberAccess[memberName] = &MemberAccessInfo{
		AccessLevel:       accessLevel,
		IsStatic:          isStatic,
		IsReadonly:        false, // Getters are not readonly in the traditional sense
		IsGetter:          true,
		IsSetter:          hasSetter,
		SetterAccessLevel: setterAccess,
		HasSetterAccess:   hasSetter && setterAccess != accessLevel,
	}
}

// AddSetterMember adds a setter method with access control information
func (cm *ClassMetadata) AddSetterMember(memberName string, accessLevel AccessModifier, isStatic bool) {
	accessLevel = privateNameAccess(memberName, accessLevel)
	hasGetter := false
	readAccess := accessLevel
	if prev := cm.MemberAccess[memberName]; prev != nil && prev.IsGetter && prev.IsStatic == isStatic {
		hasGetter = true
		readAccess = prev.AccessLevel
	}
	cm.MemberAccess[memberName] = &MemberAccessInfo{
		AccessLevel:       readAccess,
		IsStatic:          isStatic,
		IsReadonly:        false,
		IsGetter:          hasGetter,
		IsSetter:          true,
		SetterAccessLevel: accessLevel,
		HasSetterAccess:   readAccess != accessLevel,
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
	return cm.levelAccessibleFrom(memberInfo.AccessLevel, accessContext)
}

// IsWritableFrom is IsAccessibleFrom for an assignment: a get/set pair is
// written through its setter's accessibility.
func (cm *ClassMetadata) IsWritableFrom(memberName string, accessContext *AccessContext) bool {
	memberInfo := cm.GetMemberAccess(memberName)
	if memberInfo == nil {
		return false
	}
	if memberInfo.HasSetterAccess {
		return cm.levelAccessibleFrom(memberInfo.SetterAccessLevel, accessContext)
	}
	return cm.levelAccessibleFrom(memberInfo.AccessLevel, accessContext)
}

func (cm *ClassMetadata) levelAccessibleFrom(level AccessModifier, accessContext *AccessContext) bool {
	switch level {
	case AccessPublic:
		return true // Always accessible

	case AccessPrivate:
		// Only accessible within the same class (lexically: a class nested
		// in its body counts)
		return accessContext != nil && accessContext.Within(cm.SourceClassName)

	case AccessProtected:
		// Accessible within the same class or subclasses
		return accessContext != nil &&
			(accessContext.Within(cm.SourceClassName) ||
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

	// OuterClassNames are the classes whose bodies lexically enclose the
	// current one, innermost last.
	OuterClassNames []string

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

// Within reports whether the access happens lexically inside the body of
// the class className.
func (ac *AccessContext) Within(className string) bool {
	if ac.CurrentClassName == className {
		return true
	}
	for _, outer := range ac.OuterClassNames {
		if outer == className {
			return true
		}
	}
	return false
}

// privateNameAccess makes a #name member private whatever its modifiers.
func privateNameAccess(memberName string, accessLevel AccessModifier) AccessModifier {
	if len(memberName) > 0 && memberName[0] == '#' {
		return AccessPrivate
	}
	return accessLevel
}

// DisplayName is the class's declared type as diagnostics name it: `D<T>`
// for a generic class, else its name.
func (cm *ClassMetadata) DisplayName() string {
	if cm == nil {
		return ""
	}
	if cm.DisplayText != "" {
		return cm.DisplayText
	}
	if strings.HasPrefix(cm.ClassName, "__AnonymousClass_") {
		return "(Anonymous class)"
	}
	return cm.ClassName
}
